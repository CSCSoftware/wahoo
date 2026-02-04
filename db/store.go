package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store manages both the messages DB (our data) and the whatsmeow DB (session/contacts).
type Store struct {
	MsgDB *sql.DB // messages.db - our message history
	WaDB  *sql.DB // whatsapp.db - whatsmeow session + contacts
}

// NewStore opens both SQLite databases from the given directory.
// Creates the directory and tables if they don't exist.
func NewStore(storeDir string) (*Store, error) {
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %v", err)
	}

	// Open messages database
	msgPath := filepath.Join(storeDir, "messages.db")
	msgDB, err := sql.Open("sqlite3", "file:"+msgPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open messages database: %v", err)
	}

	// Create tables if they don't exist
	_, err = msgDB.Exec(`
		CREATE TABLE IF NOT EXISTS chats (
			jid TEXT PRIMARY KEY,
			name TEXT,
			last_message_time TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS messages (
			id TEXT,
			chat_jid TEXT,
			sender TEXT,
			content TEXT,
			timestamp TIMESTAMP,
			is_from_me BOOLEAN,
			media_type TEXT,
			filename TEXT,
			url TEXT,
			media_key BLOB,
			file_sha256 BLOB,
			file_enc_sha256 BLOB,
			file_length INTEGER,
			PRIMARY KEY (id, chat_jid),
			FOREIGN KEY (chat_jid) REFERENCES chats(jid)
		);
	`)
	if err != nil {
		msgDB.Close()
		return nil, fmt.Errorf("failed to create tables: %v", err)
	}

	// Open whatsmeow database (read-only for contact resolution)
	waPath := filepath.Join(storeDir, "whatsapp.db")
	waDB, err := sql.Open("sqlite3", "file:"+waPath+"?mode=ro&_journal_mode=WAL")
	if err != nil {
		// Not fatal - whatsmeow DB may not exist yet on first run
		fmt.Fprintf(os.Stderr, "Warning: could not open whatsmeow DB: %v\n", err)
		waDB = nil
	}

	return &Store{MsgDB: msgDB, WaDB: waDB}, nil
}

// Close closes both database connections.
func (s *Store) Close() {
	if s.MsgDB != nil {
		s.MsgDB.Close()
	}
	if s.WaDB != nil {
		s.WaDB.Close()
	}
}

// StoreChat upserts a chat record.
func (s *Store) StoreChat(jid, name string, lastMessageTime time.Time) error {
	_, err := s.MsgDB.Exec(
		"INSERT OR REPLACE INTO chats (jid, name, last_message_time) VALUES (?, ?, ?)",
		jid, name, lastMessageTime,
	)
	return err
}

// StoreMessage inserts or replaces a message. Skips if both content and mediaType are empty.
func (s *Store) StoreMessage(id, chatJID, sender, content string, timestamp time.Time, isFromMe bool,
	mediaType, filename, url string, mediaKey, fileSHA256, fileEncSHA256 []byte, fileLength uint64) error {

	if content == "" && mediaType == "" {
		return nil
	}

	_, err := s.MsgDB.Exec(
		`INSERT OR REPLACE INTO messages
		(id, chat_jid, sender, content, timestamp, is_from_me, media_type, filename, url, media_key, file_sha256, file_enc_sha256, file_length)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, chatJID, sender, content, timestamp, isFromMe, mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength,
	)
	return err
}

// GetMediaInfo retrieves media metadata for a message (for download).
func (s *Store) GetMediaInfo(messageID, chatJID string) (url string, mediaKey, fileSHA256, fileEncSHA256 []byte, fileLength uint64, mediaType, filename string, err error) {
	err = s.MsgDB.QueryRow(
		`SELECT url, media_key, file_sha256, file_enc_sha256, file_length, media_type, filename
		 FROM messages WHERE id = ? AND chat_jid = ?`,
		messageID, chatJID,
	).Scan(&url, &mediaKey, &fileSHA256, &fileEncSHA256, &fileLength, &mediaType, &filename)
	return
}

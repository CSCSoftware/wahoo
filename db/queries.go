package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// MessageDict is the structured output for MCP tool responses.
type MessageDict struct {
	ID        string  `json:"id"`
	Timestamp string  `json:"timestamp"`
	Sender    string  `json:"sender"`
	SenderJID string  `json:"sender_jid"`
	Content   string  `json:"content"`
	IsFromMe  bool    `json:"is_from_me"`
	ChatJID   string  `json:"chat_jid"`
	ChatName  *string `json:"chat_name,omitempty"`
	MediaType *string `json:"media_type,omitempty"`
}

// ChatDict is the structured output for chat queries.
type ChatDict struct {
	JID             string  `json:"jid"`
	Name            *string `json:"name"`
	IsGroup         bool    `json:"is_group"`
	LastMessageTime *string `json:"last_message_time,omitempty"`
	LastMessage     *string `json:"last_message,omitempty"`
	LastSender      *string `json:"last_sender,omitempty"`
	LastIsFromMe    *bool   `json:"last_is_from_me,omitempty"`
}

// ContactDict is the structured output for contact queries.
type ContactDict struct {
	PhoneNumber string  `json:"phone_number"`
	Name        *string `json:"name"`
	JID         string  `json:"jid"`
}

// MessageContextDict wraps a message with surrounding context.
type MessageContextDict struct {
	Message MessageDict   `json:"message"`
	Before  []MessageDict `json:"before"`
	After   []MessageDict `json:"after"`
}

// internal raw message from DB scan
type rawMessage struct {
	timestamp string
	sender    string
	chatName  sql.NullString
	content   sql.NullString
	isFromMe  bool
	chatJID   string
	id        string
	mediaType sql.NullString
}

// rawChat holds scanned chat data before conversion to ChatDict
type rawChat struct {
	jid          string
	name         sql.NullString
	lastTime     sql.NullString
	lastMsg      sql.NullString
	lastSender   sql.NullString
	lastIsFromMe sql.NullBool
}

// toDict converts rawChat to ChatDict with resolved last sender.
func (r rawChat) toDict(cache map[string]string) ChatDict {
	d := ChatDict{
		JID:     r.jid,
		IsGroup: strings.HasSuffix(r.jid, "@g.us"),
	}
	if r.name.Valid {
		d.Name = &r.name.String
	}
	if r.lastTime.Valid {
		d.LastMessageTime = &r.lastTime.String
	}
	if r.lastMsg.Valid {
		d.LastMessage = &r.lastMsg.String
	}
	if r.lastSender.Valid {
		senderName := resolveMessageSender(r.lastSender.String, r.lastIsFromMe.Valid && r.lastIsFromMe.Bool, cache)
		d.LastSender = &senderName
	}
	if r.lastIsFromMe.Valid {
		v := r.lastIsFromMe.Bool
		d.LastIsFromMe = &v
	}
	return d
}

// BuildSenderCache builds a JID -> display name lookup from both databases.
// Priority: whatsmeow contacts > chats table (chats often store phone numbers as names).
func (s *Store) BuildSenderCache() map[string]string {
	cache := make(map[string]string)

	// 1) Chat names from messages.db (lower priority)
	rows, err := s.MsgDB.Query("SELECT jid, name FROM chats WHERE name IS NOT NULL AND name != ''")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var jid, name string
			if rows.Scan(&jid, &name) == nil {
				cache[jid] = name
				if idx := strings.Index(jid, "@"); idx > 0 {
					cache[jid[:idx]] = name
				}
			}
		}
	}

	// 2) Contact names from whatsapp.db (higher priority - overwrites)
	if s.WaDB == nil {
		return cache
	}

	rows2, err := s.WaDB.Query("SELECT their_jid, full_name, push_name FROM whatsmeow_contacts")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read whatsmeow contacts: %v\n", err)
		return cache
	}
	defer rows2.Close()
	for rows2.Next() {
		var jid string
		var fullName, pushName sql.NullString
		if rows2.Scan(&jid, &fullName, &pushName) == nil {
			name := fullName.String
			if name == "" {
				name = pushName.String
			}
			if name != "" {
				cache[jid] = name
				if idx := strings.Index(jid, "@"); idx > 0 {
					cache[jid[:idx]] = name
				}
			}
		}
	}

	// 3) LID map: lid -> pn (phone number) -> contact name
	rows3, err := s.WaDB.Query("SELECT lid, pn FROM whatsmeow_lid_map")
	if err != nil {
		return cache
	}
	defer rows3.Close()
	for rows3.Next() {
		var lid, pn string
		if rows3.Scan(&lid, &pn) == nil {
			pnJID := pn + "@s.whatsapp.net"
			name := cache[pnJID]
			if name == "" {
				name = cache[pn]
			}
			if name != "" {
				cache[lid+"@lid"] = name
				cache[lid] = name
			}
		}
	}

	return cache
}

// resolveSender resolves a JID to a display name using the cache.
func resolveSender(senderJID string, cache map[string]string) string {
	if name, ok := cache[senderJID]; ok {
		return name
	}
	if idx := strings.Index(senderJID, "@"); idx > 0 {
		if name, ok := cache[senderJID[:idx]]; ok {
			return name
		}
	}
	return senderJID
}

// rawToDict converts a raw DB row to a MessageDict with resolved sender.
func rawToDict(r rawMessage, cache map[string]string) MessageDict {
	d := MessageDict{
		ID:        r.id,
		Timestamp: r.timestamp,
		Sender:    resolveMessageSender(r.sender, r.isFromMe, cache),
		SenderJID: r.sender,
		Content:   r.content.String,
		IsFromMe:  r.isFromMe,
		ChatJID:   r.chatJID,
	}
	if r.chatName.Valid && r.chatName.String != "" {
		d.ChatName = &r.chatName.String
	}
	if r.mediaType.Valid && r.mediaType.String != "" {
		d.MediaType = &r.mediaType.String
	}
	return d
}

// resolveMessageSender resolves a sender JID to a display name, handling "Me" for own messages.
func resolveMessageSender(senderJID string, isFromMe bool, cache map[string]string) string {
	if isFromMe {
		return "Me"
	}
	return resolveSender(senderJID, cache)
}

// ListMessagesOpts holds parameters for ListMessages.
type ListMessagesOpts struct {
	After             *string
	Before            *string
	SenderPhoneNumber *string
	ChatJID           *string
	Query             *string
	Limit             int
	Page              int
	IncludeContext    bool
	ContextBefore     int
	ContextAfter      int
}

// ListMessages returns messages matching the criteria with optional context.
func (s *Store) ListMessages(opts ListMessagesOpts) ([]MessageDict, error) {
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	if opts.IncludeContext && opts.ContextBefore == 0 {
		opts.ContextBefore = 1
	}
	if opts.IncludeContext && opts.ContextAfter == 0 {
		opts.ContextAfter = 1
	}

	queryParts := []string{
		`SELECT messages.timestamp, messages.sender, chats.name, messages.content,
		 messages.is_from_me, chats.jid, messages.id, messages.media_type
		 FROM messages
		 JOIN chats ON messages.chat_jid = chats.jid`,
	}
	var whereClauses []string
	var params []any

	if opts.After != nil {
		whereClauses = append(whereClauses, "messages.timestamp > ?")
		params = append(params, *opts.After)
	}
	if opts.Before != nil {
		whereClauses = append(whereClauses, "messages.timestamp < ?")
		params = append(params, *opts.Before)
	}
	if opts.SenderPhoneNumber != nil {
		whereClauses = append(whereClauses, "messages.sender = ?")
		params = append(params, *opts.SenderPhoneNumber)
	}
	if opts.ChatJID != nil {
		whereClauses = append(whereClauses, "messages.chat_jid = ?")
		params = append(params, *opts.ChatJID)
	}
	if opts.Query != nil {
		whereClauses = append(whereClauses, "(LOWER(messages.content) LIKE LOWER(?) OR LOWER(messages.media_type) LIKE LOWER(?))")
		q := "%" + *opts.Query + "%"
		params = append(params, q, q)
	}

	if len(whereClauses) > 0 {
		queryParts = append(queryParts, "WHERE "+strings.Join(whereClauses, " AND "))
	}

	offset := opts.Page * opts.Limit
	queryParts = append(queryParts, "ORDER BY messages.timestamp DESC")
	queryParts = append(queryParts, "LIMIT ? OFFSET ?")
	params = append(params, opts.Limit, offset)

	rows, err := s.MsgDB.Query(strings.Join(queryParts, " "), params...)
	if err != nil {
		return nil, fmt.Errorf("list messages query: %w", err)
	}
	defer rows.Close()

	var messages []rawMessage
	for rows.Next() {
		var m rawMessage
		if err := rows.Scan(&m.timestamp, &m.sender, &m.chatName, &m.content,
			&m.isFromMe, &m.chatJID, &m.id, &m.mediaType); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, m)
	}

	cache := s.BuildSenderCache()

	if opts.IncludeContext && len(messages) > 0 {
		var result []MessageDict
		seen := make(map[string]bool)
		for _, msg := range messages {
			ctx, err := s.getMessageContextRaw(msg.id, opts.ContextBefore, opts.ContextAfter)
			if err != nil {
				continue
			}
			for _, m := range ctx {
				if !seen[m.id] {
					seen[m.id] = true
					result = append(result, rawToDict(m, cache))
				}
			}
		}
		return result, nil
	}

	result := make([]MessageDict, 0, len(messages))
	for _, m := range messages {
		result = append(result, rawToDict(m, cache))
	}
	return result, nil
}

// getMessageContextRaw returns before + target + after as raw messages.
func (s *Store) getMessageContextRaw(messageID string, before, after int) ([]rawMessage, error) {
	// Get target message
	var target rawMessage
	var chatJID string
	err := s.MsgDB.QueryRow(
		`SELECT messages.timestamp, messages.sender, chats.name, messages.content,
		 messages.is_from_me, chats.jid, messages.id, messages.chat_jid, messages.media_type
		 FROM messages JOIN chats ON messages.chat_jid = chats.jid
		 WHERE messages.id = ?`, messageID,
	).Scan(&target.timestamp, &target.sender, &target.chatName, &target.content,
		&target.isFromMe, &target.chatJID, &target.id, &chatJID, &target.mediaType)
	if err != nil {
		return nil, fmt.Errorf("message %s not found: %w", messageID, err)
	}

	var result []rawMessage

	// Messages before
	rows, err := s.MsgDB.Query(
		`SELECT messages.timestamp, messages.sender, chats.name, messages.content,
		 messages.is_from_me, chats.jid, messages.id, messages.media_type
		 FROM messages JOIN chats ON messages.chat_jid = chats.jid
		 WHERE messages.chat_jid = ? AND messages.timestamp < ?
		 ORDER BY messages.timestamp DESC LIMIT ?`,
		chatJID, target.timestamp, before,
	)
	if err == nil {
		defer rows.Close()
		var beforeMsgs []rawMessage
		for rows.Next() {
			var m rawMessage
			rows.Scan(&m.timestamp, &m.sender, &m.chatName, &m.content,
				&m.isFromMe, &m.chatJID, &m.id, &m.mediaType)
			beforeMsgs = append(beforeMsgs, m)
		}
		// Reverse to chronological order
		for i := len(beforeMsgs) - 1; i >= 0; i-- {
			result = append(result, beforeMsgs[i])
		}
	}

	result = append(result, target)

	// Messages after
	rows2, err := s.MsgDB.Query(
		`SELECT messages.timestamp, messages.sender, chats.name, messages.content,
		 messages.is_from_me, chats.jid, messages.id, messages.media_type
		 FROM messages JOIN chats ON messages.chat_jid = chats.jid
		 WHERE messages.chat_jid = ? AND messages.timestamp > ?
		 ORDER BY messages.timestamp ASC LIMIT ?`,
		chatJID, target.timestamp, after,
	)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var m rawMessage
			rows2.Scan(&m.timestamp, &m.sender, &m.chatName, &m.content,
				&m.isFromMe, &m.chatJID, &m.id, &m.mediaType)
			result = append(result, m)
		}
	}

	return result, nil
}

// GetMessageContext returns a message with surrounding context as structured dicts.
func (s *Store) GetMessageContext(messageID string, before, after int) (*MessageContextDict, error) {
	if before == 0 {
		before = 5
	}
	if after == 0 {
		after = 5
	}

	// Get target
	var target rawMessage
	var chatJID string
	err := s.MsgDB.QueryRow(
		`SELECT messages.timestamp, messages.sender, chats.name, messages.content,
		 messages.is_from_me, chats.jid, messages.id, messages.chat_jid, messages.media_type
		 FROM messages JOIN chats ON messages.chat_jid = chats.jid
		 WHERE messages.id = ?`, messageID,
	).Scan(&target.timestamp, &target.sender, &target.chatName, &target.content,
		&target.isFromMe, &target.chatJID, &target.id, &chatJID, &target.mediaType)
	if err != nil {
		return nil, fmt.Errorf("message %s not found: %w", messageID, err)
	}

	cache := s.BuildSenderCache()
	result := &MessageContextDict{
		Message: rawToDict(target, cache),
	}

	// Before
	rows, err := s.MsgDB.Query(
		`SELECT messages.timestamp, messages.sender, chats.name, messages.content,
		 messages.is_from_me, chats.jid, messages.id, messages.media_type
		 FROM messages JOIN chats ON messages.chat_jid = chats.jid
		 WHERE messages.chat_jid = ? AND messages.timestamp < ?
		 ORDER BY messages.timestamp DESC LIMIT ?`,
		chatJID, target.timestamp, before,
	)
	if err == nil {
		defer rows.Close()
		var beforeMsgs []MessageDict
		for rows.Next() {
			var m rawMessage
			rows.Scan(&m.timestamp, &m.sender, &m.chatName, &m.content,
				&m.isFromMe, &m.chatJID, &m.id, &m.mediaType)
			beforeMsgs = append(beforeMsgs, rawToDict(m, cache))
		}
		// Reverse to chronological order
		for i, j := 0, len(beforeMsgs)-1; i < j; i, j = i+1, j-1 {
			beforeMsgs[i], beforeMsgs[j] = beforeMsgs[j], beforeMsgs[i]
		}
		result.Before = beforeMsgs
	}
	if result.Before == nil {
		result.Before = []MessageDict{}
	}

	// After
	rows2, err := s.MsgDB.Query(
		`SELECT messages.timestamp, messages.sender, chats.name, messages.content,
		 messages.is_from_me, chats.jid, messages.id, messages.media_type
		 FROM messages JOIN chats ON messages.chat_jid = chats.jid
		 WHERE messages.chat_jid = ? AND messages.timestamp > ?
		 ORDER BY messages.timestamp ASC LIMIT ?`,
		chatJID, target.timestamp, after,
	)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var m rawMessage
			rows2.Scan(&m.timestamp, &m.sender, &m.chatName, &m.content,
				&m.isFromMe, &m.chatJID, &m.id, &m.mediaType)
			result.After = append(result.After, rawToDict(m, cache))
		}
	}
	if result.After == nil {
		result.After = []MessageDict{}
	}

	return result, nil
}

// ListChatsOpts holds parameters for ListChats.
type ListChatsOpts struct {
	Query              *string
	Limit              int
	Page               int
	IncludeLastMessage bool
	SortBy             string // "last_active" or "name"
}

// ListChats returns chats matching the criteria.
func (s *Store) ListChats(opts ListChatsOpts) ([]ChatDict, error) {
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	if opts.SortBy == "" {
		opts.SortBy = "last_active"
	}

	queryParts := []string{
		`SELECT chats.jid, chats.name, chats.last_message_time,
		 messages.content, messages.sender, messages.is_from_me
		 FROM chats`,
	}

	if opts.IncludeLastMessage {
		queryParts = append(queryParts,
			`LEFT JOIN messages ON chats.jid = messages.chat_jid
			 AND chats.last_message_time = messages.timestamp`)
	}

	var whereClauses []string
	var params []any

	if opts.Query != nil {
		whereClauses = append(whereClauses, "(LOWER(chats.name) LIKE LOWER(?) OR chats.jid LIKE ?)")
		q := "%" + *opts.Query + "%"
		params = append(params, q, q)
	}

	if len(whereClauses) > 0 {
		queryParts = append(queryParts, "WHERE "+strings.Join(whereClauses, " AND "))
	}

	if opts.SortBy == "last_active" {
		queryParts = append(queryParts, "ORDER BY chats.last_message_time DESC")
	} else {
		queryParts = append(queryParts, "ORDER BY chats.name")
	}

	offset := opts.Page * opts.Limit
	queryParts = append(queryParts, "LIMIT ? OFFSET ?")
	params = append(params, opts.Limit, offset)

	rows, err := s.MsgDB.Query(strings.Join(queryParts, " "), params...)
	if err != nil {
		return nil, fmt.Errorf("list chats query: %w", err)
	}
	defer rows.Close()

	cache := s.BuildSenderCache()
	var result []ChatDict

	for rows.Next() {
		var r rawChat
		if err := rows.Scan(&r.jid, &r.name, &r.lastTime, &r.lastMsg, &r.lastSender, &r.lastIsFromMe); err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		result = append(result, r.toDict(cache))
	}

	if result == nil {
		result = []ChatDict{}
	}
	return result, nil
}

// SearchContacts searches for contacts by name or phone number.
func (s *Store) SearchContacts(query string) ([]ContactDict, error) {
	pattern := "%" + query + "%"
	rows, err := s.MsgDB.Query(`
		SELECT DISTINCT jid, name FROM chats
		WHERE (LOWER(name) LIKE LOWER(?) OR LOWER(jid) LIKE LOWER(?))
		AND jid NOT LIKE '%@g.us'
		ORDER BY name, jid
		LIMIT 50`,
		pattern, pattern,
	)
	if err != nil {
		return nil, fmt.Errorf("search contacts: %w", err)
	}
	defer rows.Close()

	var result []ContactDict
	for rows.Next() {
		var jid string
		var name sql.NullString
		if err := rows.Scan(&jid, &name); err != nil {
			continue
		}
		phone := jid
		if idx := strings.Index(jid, "@"); idx > 0 {
			phone = jid[:idx]
		}
		d := ContactDict{
			PhoneNumber: phone,
			JID:         jid,
		}
		if name.Valid {
			d.Name = &name.String
		}
		result = append(result, d)
	}

	if result == nil {
		result = []ContactDict{}
	}
	return result, nil
}

// GetChat returns a single chat by JID.
func (s *Store) GetChat(chatJID string, includeLastMessage bool) (*ChatDict, error) {
	q := `SELECT c.jid, c.name, c.last_message_time,
		  m.content, m.sender, m.is_from_me
		  FROM chats c`

	if includeLastMessage {
		q += ` LEFT JOIN messages m ON c.jid = m.chat_jid
			   AND c.last_message_time = m.timestamp`
	}
	q += " WHERE c.jid = ?"

	var r rawChat
	err := s.MsgDB.QueryRow(q, chatJID).Scan(&r.jid, &r.name, &r.lastTime, &r.lastMsg, &r.lastSender, &r.lastIsFromMe)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get chat: %w", err)
	}

	cache := s.BuildSenderCache()
	d := r.toDict(cache)
	return &d, nil
}

// GetDirectChatByContact finds a direct chat by phone number.
func (s *Store) GetDirectChatByContact(phoneNumber string) (*ChatDict, error) {
	q := `SELECT c.jid, c.name, c.last_message_time,
		  m.content, m.sender, m.is_from_me
		  FROM chats c
		  LEFT JOIN messages m ON c.jid = m.chat_jid AND c.last_message_time = m.timestamp
		  WHERE c.jid LIKE ? AND c.jid NOT LIKE '%@g.us'
		  LIMIT 1`

	var r rawChat
	err := s.MsgDB.QueryRow(q, "%"+phoneNumber+"%").Scan(&r.jid, &r.name, &r.lastTime, &r.lastMsg, &r.lastSender, &r.lastIsFromMe)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get direct chat: %w", err)
	}

	cache := s.BuildSenderCache()
	d := r.toDict(cache)
	return &d, nil
}

// GetContactChats returns all chats involving a contact.
func (s *Store) GetContactChats(jid string, limit, page int) ([]ChatDict, error) {
	if limit == 0 {
		limit = 20
	}

	rows, err := s.MsgDB.Query(`
		SELECT DISTINCT c.jid, c.name, c.last_message_time,
		 m.content, m.sender, m.is_from_me
		FROM chats c
		JOIN messages m ON c.jid = m.chat_jid
		WHERE m.sender = ? OR c.jid = ?
		ORDER BY c.last_message_time DESC
		LIMIT ? OFFSET ?`,
		jid, jid, limit, page*limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get contact chats: %w", err)
	}
	defer rows.Close()

	cache := s.BuildSenderCache()
	var result []ChatDict

	for rows.Next() {
		var r rawChat
		if err := rows.Scan(&r.jid, &r.name, &r.lastTime, &r.lastMsg, &r.lastSender, &r.lastIsFromMe); err != nil {
			continue
		}
		result = append(result, r.toDict(cache))
	}

	if result == nil {
		result = []ChatDict{}
	}
	return result, nil
}

// GetLastInteraction returns the most recent message involving a contact.
func (s *Store) GetLastInteraction(jid string) (*MessageDict, error) {
	var m rawMessage
	err := s.MsgDB.QueryRow(`
		SELECT m.timestamp, m.sender, c.name, m.content, m.is_from_me, c.jid, m.id, m.media_type
		FROM messages m
		JOIN chats c ON m.chat_jid = c.jid
		WHERE m.sender = ? OR c.jid = ?
		ORDER BY m.timestamp DESC LIMIT 1`,
		jid, jid,
	).Scan(&m.timestamp, &m.sender, &m.chatName, &m.content,
		&m.isFromMe, &m.chatJID, &m.id, &m.mediaType)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get last interaction: %w", err)
	}

	cache := s.BuildSenderCache()
	d := rawToDict(m, cache)
	return &d, nil
}


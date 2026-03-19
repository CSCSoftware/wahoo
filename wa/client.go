package wa

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"github.com/mdp/qrterminal"

	"github.com/CSCSoftware/wahoo/db"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// stderrLogger implements waLog.Logger but writes to stderr instead of stdout.
type stderrLogger struct {
	module   string
	minLevel int
	color    bool
}

var levelMap = map[string]int{"DEBUG": 0, "INFO": 1, "WARN": 2, "ERROR": 3}

func newStderrLogger(module, minLevel string, color bool) waLog.Logger {
	lvl, ok := levelMap[strings.ToUpper(minLevel)]
	if !ok {
		lvl = 1
	}
	return &stderrLogger{module: module, minLevel: lvl, color: color}
}

func (l *stderrLogger) log(level string, lvl int, msg string, args ...interface{}) {
	if lvl < l.minLevel {
		return
	}
	text := fmt.Sprintf(msg, args...)
	if l.color {
		colors := map[string]string{"DEBUG": "\033[37m", "INFO": "\033[36m", "WARN": "\033[33m", "ERROR": "\033[31m"}
		fmt.Fprintf(os.Stderr, "%s%s [%s %s] %s\033[0m\n", colors[level], time.Now().Format("15:04:05.000"), l.module, level, text)
	} else {
		fmt.Fprintf(os.Stderr, "%s [%s %s] %s\n", time.Now().Format("15:04:05.000"), l.module, level, text)
	}
}

func (l *stderrLogger) Debugf(msg string, args ...interface{}) { l.log("DEBUG", 0, msg, args...) }
func (l *stderrLogger) Infof(msg string, args ...interface{})  { l.log("INFO", 1, msg, args...) }
func (l *stderrLogger) Warnf(msg string, args ...interface{})  { l.log("WARN", 2, msg, args...) }
func (l *stderrLogger) Errorf(msg string, args ...interface{}) { l.log("ERROR", 3, msg, args...) }
func (l *stderrLogger) Sub(module string) waLog.Logger {
	return &stderrLogger{module: l.module + "/" + module, minLevel: l.minLevel, color: l.color}
}

// Client wraps the whatsmeow client and our message store.
type Client struct {
	WA       *whatsmeow.Client
	Store    *db.Store
	StoreDir string
	Logger   waLog.Logger
}

// NewClient creates a new WhatsApp client and connects to the whatsmeow session DB.
func NewClient(store *db.Store, storeDir string) (*Client, error) {
	// All whatsmeow logs go to stderr (stdout is for MCP)
	logger := newStderrLogger("WhatsApp", "INFO", true)

	// Open whatsmeow session container
	dbPath := filepath.Join(storeDir, "whatsapp.db")
	dbLog := newStderrLogger("Database", "INFO", true)
	container, err := sqlstore.New(context.Background(), "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)", dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to open whatsmeow DB: %w", err)
	}

	// Get or create device
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		if err == sql.ErrNoRows {
			deviceStore = container.NewDevice()
			logger.Infof("Created new device")
		} else {
			return nil, fmt.Errorf("failed to get device: %w", err)
		}
	}

	waClient := whatsmeow.NewClient(deviceStore, logger)
	if waClient == nil {
		return nil, fmt.Errorf("failed to create WhatsApp client")
	}

	return &Client{
		WA:       waClient,
		Store:    store,
		StoreDir: storeDir,
		Logger:   logger,
	}, nil
}

// Connect connects to WhatsApp, showing QR code on stderr if needed.
func (c *Client) Connect(ctx context.Context) error {
	// Register event handlers
	c.WA.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			handleMessage(c, v)
		case *events.HistorySync:
			handleHistorySync(c, v)
		case *events.Connected:
			c.Logger.Infof("Connected to WhatsApp")
		case *events.LoggedOut:
			c.Logger.Warnf("Device logged out")
		}
	})

	if c.WA.Store.ID == nil {
		// New client - need QR code pairing
		qrChan, _ := c.WA.GetQRChannel(ctx)
		if err := c.WA.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}

		// QR code goes to stderr (stdout is MCP)
		connected := make(chan bool, 1)
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Fprintln(os.Stderr, "\nScan this QR code with your WhatsApp app:")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stderr)
			} else if evt.Event == "success" {
				connected <- true
				break
			}
		}

		select {
		case <-connected:
			fmt.Fprintln(os.Stderr, "Successfully connected and authenticated!")
		case <-time.After(3 * time.Minute):
			return fmt.Errorf("timeout waiting for QR code scan")
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		// Already logged in
		if err := c.WA.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
	}

	// Wait for connection to stabilize
	time.Sleep(2 * time.Second)

	if !c.WA.IsConnected() {
		return fmt.Errorf("failed to establish stable connection")
	}

	fmt.Fprintln(os.Stderr, "WhatsApp connected.")
	return nil
}

// Disconnect cleanly disconnects from WhatsApp.
func (c *Client) Disconnect() {
	if c.WA != nil {
		c.WA.Disconnect()
	}
}

// IsConnected returns whether the client is connected to WhatsApp.
func (c *Client) IsConnected() bool {
	return c.WA != nil && c.WA.IsConnected()
}

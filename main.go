package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/CSCSoftware/wahoo/db"
	mcpServer "github.com/CSCSoftware/wahoo/mcp"
	"github.com/CSCSoftware/wahoo/wa"
)

func main() {
	storeDir := flag.String("store-dir", "store", "Directory for SQLite databases")
	flag.Parse()

	// All non-MCP output goes to stderr
	fmt.Fprintln(os.Stderr, "wahoo - WhatsApp MCP Server")
	fmt.Fprintf(os.Stderr, "Store directory: %s\n", *storeDir)

	// Open databases
	store, err := db.NewStore(*storeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open databases: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// Create and connect WhatsApp client
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := wa.NewClient(store, *storeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create WhatsApp client: %v\n", err)
		os.Exit(1)
	}

	// Connect in background goroutine
	go func() {
		if err := client.Connect(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "WhatsApp connection error: %v\n", err)
			// Don't exit - MCP server can still serve read-only DB queries
		}
	}()

	// Handle OS signals for clean shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		fmt.Fprintln(os.Stderr, "Shutting down...")
		cancel()
		client.Disconnect()
		os.Exit(0)
	}()

	// Create and run MCP server (blocks on stdin/stdout)
	server := mcpServer.NewServer(store, client)
	if err := server.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

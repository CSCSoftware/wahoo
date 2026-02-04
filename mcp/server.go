package mcp

import (
	"context"

	"github.com/CSCSoftware/wahoo/db"
	"github.com/CSCSoftware/wahoo/wa"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP server with our store and WhatsApp client.
type Server struct {
	mcpServer *mcp.Server
	store     *db.Store
	client    *wa.Client
}

// NewServer creates an MCP server with all WhatsApp tools registered.
func NewServer(store *db.Store, client *wa.Client) *Server {
	s := &Server{
		store:  store,
		client: client,
	}

	s.mcpServer = mcp.NewServer(&mcp.Implementation{
		Name:    "whatsapp",
		Version: "1.0.0",
	}, nil)

	s.registerTools()
	return s
}

// Run starts the MCP server on stdio (blocking).
func (s *Server) Run(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &mcp.StdioTransport{})
}

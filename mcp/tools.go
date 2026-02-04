package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/CSCSoftware/wahoo/db"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools registers all 19 WhatsApp MCP tools.
func (s *Server) registerTools() {
	// === Read-only DB tools (no WhatsApp client needed) ===

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "search_contacts",
		Description: "Search WhatsApp contacts by name or phone number.",
	}, s.handleSearchContacts)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_messages",
		Description: "Get WhatsApp messages matching specified criteria with optional context.",
	}, s.handleListMessages)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_chats",
		Description: "Get WhatsApp chats matching specified criteria.",
	}, s.handleListChats)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_chat",
		Description: "Get WhatsApp chat metadata by JID.",
	}, s.handleGetChat)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_direct_chat_by_contact",
		Description: "Get WhatsApp chat metadata by sender phone number.",
	}, s.handleGetDirectChatByContact)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_contact_chats",
		Description: "Get all WhatsApp chats involving the contact.",
	}, s.handleGetContactChats)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_last_interaction",
		Description: "Get most recent WhatsApp message involving the contact.",
	}, s.handleGetLastInteraction)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_message_context",
		Description: "Get context around a specific WhatsApp message.",
	}, s.handleGetMessageContext)

	// === Write tools (need WhatsApp client) ===

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "send_message",
		Description: "Send a WhatsApp message to a person or group. For group chats use the JID.",
	}, s.handleSendMessage)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "send_file",
		Description: "Send a file such as a picture, raw audio, video or document via WhatsApp. For group messages use the JID.",
	}, s.handleSendFile)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "send_audio_message",
		Description: "Send any audio file as a WhatsApp audio message. If it errors due to ffmpeg not being installed, use send_file instead.",
	}, s.handleSendAudioMessage)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "download_media",
		Description: "Download media from a WhatsApp message and get the local file path.",
	}, s.handleDownloadMedia)

	// === Chat management tools ===

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "revoke_message",
		Description: "Delete/revoke a WhatsApp message. Can revoke own messages or others' messages as group admin.",
	}, s.handleRevokeMessage)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "block_contact",
		Description: "Block a WhatsApp contact.",
	}, s.handleBlockContact)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "unblock_contact",
		Description: "Unblock a previously blocked WhatsApp contact.",
	}, s.handleUnblockContact)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_blocklist",
		Description: "Get the list of all blocked WhatsApp contacts.",
	}, s.handleGetBlocklist)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "mute_chat",
		Description: "Mute or unmute a WhatsApp chat. Duration in hours, 0 = mute forever.",
	}, s.handleMuteChat)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "pin_chat",
		Description: "Pin or unpin a WhatsApp chat.",
	}, s.handlePinChat)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "archive_chat",
		Description: "Archive or unarchive a WhatsApp chat.",
	}, s.handleArchiveChat)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_chat",
		Description: "Delete a WhatsApp chat entirely (removes from WhatsApp and local DB).",
	}, s.handleDeleteChat)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "mark_chat_read",
		Description: "Mark a WhatsApp chat as read or unread.",
	}, s.handleMarkChatRead)
}

// --- Input types ---

type searchContactsInput struct {
	Query string `json:"query" jsonschema:"Search term to match against contact names or phone numbers"`
}

type listMessagesInput struct {
	After             string `json:"after,omitempty" jsonschema:"ISO-8601 date to only return messages after"`
	Before            string `json:"before,omitempty" jsonschema:"ISO-8601 date to only return messages before"`
	SenderPhoneNumber string `json:"sender_phone_number,omitempty" jsonschema:"Phone number to filter by sender"`
	ChatJID           string `json:"chat_jid,omitempty" jsonschema:"Chat JID to filter messages"`
	Query             string `json:"query,omitempty" jsonschema:"Search term to filter messages by content"`
	Limit             int    `json:"limit,omitempty" jsonschema:"Maximum number of messages (default 20)"`
	Page              int    `json:"page,omitempty" jsonschema:"Page number for pagination (default 0)"`
	IncludeContext    *bool  `json:"include_context,omitempty" jsonschema:"Include surrounding context messages (default true)"`
	ContextBefore     int    `json:"context_before,omitempty" jsonschema:"Number of messages before each match (default 1)"`
	ContextAfter      int    `json:"context_after,omitempty" jsonschema:"Number of messages after each match (default 1)"`
}

type listChatsInput struct {
	Query              string `json:"query,omitempty" jsonschema:"Search term to filter chats by name or JID"`
	Limit              int    `json:"limit,omitempty" jsonschema:"Maximum number of chats (default 20)"`
	Page               int    `json:"page,omitempty" jsonschema:"Page number for pagination (default 0)"`
	IncludeLastMessage *bool  `json:"include_last_message,omitempty" jsonschema:"Include last message in each chat (default true)"`
	SortBy             string `json:"sort_by,omitempty" jsonschema:"Sort by last_active or name (default last_active)"`
}

type getChatInput struct {
	ChatJID            string `json:"chat_jid" jsonschema:"The JID of the chat to retrieve"`
	IncludeLastMessage *bool  `json:"include_last_message,omitempty" jsonschema:"Include last message (default true)"`
}

type getDirectChatByContactInput struct {
	SenderPhoneNumber string `json:"sender_phone_number" jsonschema:"The phone number to search for"`
}

type getContactChatsInput struct {
	JID   string `json:"jid" jsonschema:"The contact's JID to search for"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum chats to return (default 20)"`
	Page  int    `json:"page,omitempty" jsonschema:"Page number (default 0)"`
}

type getLastInteractionInput struct {
	JID string `json:"jid" jsonschema:"The JID of the contact to search for"`
}

type getMessageContextInput struct {
	MessageID string `json:"message_id" jsonschema:"The ID of the message to get context for"`
	Before    int    `json:"before,omitempty" jsonschema:"Number of messages before (default 5)"`
	After     int    `json:"after,omitempty" jsonschema:"Number of messages after (default 5)"`
}

type sendMessageInput struct {
	Recipient string `json:"recipient" jsonschema:"Phone number (no + or symbols) or JID"`
	Message   string `json:"message" jsonschema:"The message text to send"`
}

type sendFileInput struct {
	Recipient string `json:"recipient" jsonschema:"Phone number (no + or symbols) or JID"`
	MediaPath string `json:"media_path" jsonschema:"Absolute path to the media file to send"`
}

type sendAudioMessageInput struct {
	Recipient string `json:"recipient" jsonschema:"Phone number (no + or symbols) or JID"`
	MediaPath string `json:"media_path" jsonschema:"Absolute path to the audio file"`
}

type downloadMediaInput struct {
	MessageID string `json:"message_id" jsonschema:"ID of the message containing the media"`
	ChatJID   string `json:"chat_jid" jsonschema:"JID of the chat containing the message"`
}

type revokeMessageInput struct {
	ChatJID   string `json:"chat_jid" jsonschema:"JID of the chat containing the message"`
	MessageID string `json:"message_id" jsonschema:"ID of the message to revoke/delete"`
	SenderJID string `json:"sender_jid,omitempty" jsonschema:"Sender JID (only needed to revoke others messages as group admin)"`
}

type blockContactInput struct {
	JID string `json:"jid" jsonschema:"JID of the contact to block (e.g. 491234567890@s.whatsapp.net)"`
}

type unblockContactInput struct {
	JID string `json:"jid" jsonschema:"JID of the contact to unblock"`
}

type emptyInput struct{}

type muteChatInput struct {
	ChatJID       string `json:"chat_jid" jsonschema:"JID of the chat to mute/unmute"`
	Mute          bool   `json:"mute" jsonschema:"true to mute, false to unmute"`
	DurationHours int    `json:"duration_hours,omitempty" jsonschema:"Mute duration in hours (0 = forever, only used when mute=true)"`
}

type pinChatInput struct {
	ChatJID string `json:"chat_jid" jsonschema:"JID of the chat to pin/unpin"`
	Pin     bool   `json:"pin" jsonschema:"true to pin, false to unpin"`
}

type archiveChatInput struct {
	ChatJID string `json:"chat_jid" jsonschema:"JID of the chat to archive/unarchive"`
	Archive bool   `json:"archive" jsonschema:"true to archive, false to unarchive"`
}

type deleteChatInput struct {
	ChatJID string `json:"chat_jid" jsonschema:"JID of the chat to delete"`
}

type markChatReadInput struct {
	ChatJID string `json:"chat_jid" jsonschema:"JID of the chat to mark"`
	Read    bool   `json:"read" jsonschema:"true to mark as read, false to mark as unread"`
}

// --- Output wrapper types (MCP SDK requires type "object", not slices/pointers) ---

type contactsResult struct {
	Contacts []db.ContactDict `json:"contacts"`
	Count    int              `json:"count"`
}

type messagesResult struct {
	Messages []db.MessageDict `json:"messages"`
	Count    int              `json:"count"`
}

type chatsResult struct {
	Chats []db.ChatDict `json:"chats"`
	Count int           `json:"count"`
}

type chatResult struct {
	Chat db.ChatDict `json:"chat"`
}

type messageResult struct {
	Message db.MessageDict `json:"message"`
}

type messageContextResult struct {
	Context db.MessageContextDict `json:"context"`
}

// --- Handlers ---

func (s *Server) handleSearchContacts(ctx context.Context, req *mcp.CallToolRequest, input searchContactsInput) (*mcp.CallToolResult, contactsResult, error) {
	result, err := s.store.SearchContacts(input.Query)
	if err != nil {
		return nil, contactsResult{}, err
	}
	if result == nil {
		result = []db.ContactDict{}
	}
	return nil, contactsResult{Contacts: result, Count: len(result)}, nil
}

func (s *Server) handleListMessages(ctx context.Context, req *mcp.CallToolRequest, input listMessagesInput) (*mcp.CallToolResult, messagesResult, error) {
	opts := db.ListMessagesOpts{
		Limit:          input.Limit,
		Page:           input.Page,
		IncludeContext: true,
		ContextBefore:  input.ContextBefore,
		ContextAfter:   input.ContextAfter,
	}
	if input.After != "" {
		opts.After = &input.After
	}
	if input.Before != "" {
		opts.Before = &input.Before
	}
	if input.SenderPhoneNumber != "" {
		opts.SenderPhoneNumber = &input.SenderPhoneNumber
	}
	if input.ChatJID != "" {
		opts.ChatJID = &input.ChatJID
	}
	if input.Query != "" {
		opts.Query = &input.Query
	}
	if input.IncludeContext != nil {
		opts.IncludeContext = *input.IncludeContext
	}

	result, err := s.store.ListMessages(opts)
	if err != nil {
		return nil, messagesResult{}, err
	}
	if result == nil {
		result = []db.MessageDict{}
	}
	return nil, messagesResult{Messages: result, Count: len(result)}, nil
}

func (s *Server) handleListChats(ctx context.Context, req *mcp.CallToolRequest, input listChatsInput) (*mcp.CallToolResult, chatsResult, error) {
	opts := db.ListChatsOpts{
		Limit:              input.Limit,
		Page:               input.Page,
		IncludeLastMessage: true,
		SortBy:             input.SortBy,
	}
	if input.Query != "" {
		opts.Query = &input.Query
	}
	if input.IncludeLastMessage != nil {
		opts.IncludeLastMessage = *input.IncludeLastMessage
	}

	result, err := s.store.ListChats(opts)
	if err != nil {
		return nil, chatsResult{}, err
	}
	if result == nil {
		result = []db.ChatDict{}
	}
	return nil, chatsResult{Chats: result, Count: len(result)}, nil
}

func (s *Server) handleGetChat(ctx context.Context, req *mcp.CallToolRequest, input getChatInput) (*mcp.CallToolResult, chatResult, error) {
	includeLastMsg := true
	if input.IncludeLastMessage != nil {
		includeLastMsg = *input.IncludeLastMessage
	}
	result, err := s.store.GetChat(input.ChatJID, includeLastMsg)
	if err != nil {
		return nil, chatResult{}, err
	}
	if result == nil {
		return nil, chatResult{}, fmt.Errorf("chat not found: %s", input.ChatJID)
	}
	return nil, chatResult{Chat: *result}, nil
}

func (s *Server) handleGetDirectChatByContact(ctx context.Context, req *mcp.CallToolRequest, input getDirectChatByContactInput) (*mcp.CallToolResult, chatResult, error) {
	result, err := s.store.GetDirectChatByContact(input.SenderPhoneNumber)
	if err != nil {
		return nil, chatResult{}, err
	}
	if result == nil {
		return nil, chatResult{}, fmt.Errorf("no direct chat found for: %s", input.SenderPhoneNumber)
	}
	return nil, chatResult{Chat: *result}, nil
}

func (s *Server) handleGetContactChats(ctx context.Context, req *mcp.CallToolRequest, input getContactChatsInput) (*mcp.CallToolResult, chatsResult, error) {
	result, err := s.store.GetContactChats(input.JID, input.Limit, input.Page)
	if err != nil {
		return nil, chatsResult{}, err
	}
	if result == nil {
		result = []db.ChatDict{}
	}
	return nil, chatsResult{Chats: result, Count: len(result)}, nil
}

func (s *Server) handleGetLastInteraction(ctx context.Context, req *mcp.CallToolRequest, input getLastInteractionInput) (*mcp.CallToolResult, messageResult, error) {
	result, err := s.store.GetLastInteraction(input.JID)
	if err != nil {
		return nil, messageResult{}, err
	}
	if result == nil {
		return nil, messageResult{}, fmt.Errorf("no interaction found for: %s", input.JID)
	}
	return nil, messageResult{Message: *result}, nil
}

func (s *Server) handleGetMessageContext(ctx context.Context, req *mcp.CallToolRequest, input getMessageContextInput) (*mcp.CallToolResult, messageContextResult, error) {
	result, err := s.store.GetMessageContext(input.MessageID, input.Before, input.After)
	if err != nil {
		return nil, messageContextResult{}, err
	}
	if result == nil {
		return nil, messageContextResult{}, fmt.Errorf("message not found: %s", input.MessageID)
	}
	return nil, messageContextResult{Context: *result}, nil
}

type sendResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (s *Server) handleSendMessage(ctx context.Context, req *mcp.CallToolRequest, input sendMessageInput) (*mcp.CallToolResult, sendResult, error) {
	if input.Recipient == "" {
		return nil, sendResult{Success: false, Message: "Recipient must be provided"}, nil
	}
	if s.client == nil {
		return nil, sendResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	success, msg := s.client.SendMessage(input.Recipient, input.Message)
	return nil, sendResult{Success: success, Message: msg}, nil
}

func (s *Server) handleSendFile(ctx context.Context, req *mcp.CallToolRequest, input sendFileInput) (*mcp.CallToolResult, sendResult, error) {
	if input.Recipient == "" {
		return nil, sendResult{Success: false, Message: "Recipient must be provided"}, nil
	}
	if s.client == nil {
		return nil, sendResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	success, msg := s.client.SendMedia(input.Recipient, input.MediaPath, "")
	return nil, sendResult{Success: success, Message: msg}, nil
}

func (s *Server) handleSendAudioMessage(ctx context.Context, req *mcp.CallToolRequest, input sendAudioMessageInput) (*mcp.CallToolResult, sendResult, error) {
	if input.Recipient == "" {
		return nil, sendResult{Success: false, Message: "Recipient must be provided"}, nil
	}
	if s.client == nil {
		return nil, sendResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	success, msg := s.client.SendAudioMessage(input.Recipient, input.MediaPath)
	return nil, sendResult{Success: success, Message: msg}, nil
}

type downloadResult struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	FilePath string `json:"file_path,omitempty"`
}

func (s *Server) handleDownloadMedia(ctx context.Context, req *mcp.CallToolRequest, input downloadMediaInput) (*mcp.CallToolResult, downloadResult, error) {
	if s.client == nil {
		return nil, downloadResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	path, err := s.client.DownloadMedia(input.MessageID, input.ChatJID)
	if err != nil {
		return nil, downloadResult{Success: false, Message: err.Error()}, nil
	}
	return nil, downloadResult{Success: true, Message: "Media downloaded successfully", FilePath: path}, nil
}

// --- Chat management handlers ---

func (s *Server) handleRevokeMessage(ctx context.Context, req *mcp.CallToolRequest, input revokeMessageInput) (*mcp.CallToolResult, sendResult, error) {
	if s.client == nil {
		return nil, sendResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	success, msg := s.client.RevokeMessage(input.ChatJID, input.MessageID, input.SenderJID)
	return nil, sendResult{Success: success, Message: msg}, nil
}

func (s *Server) handleBlockContact(ctx context.Context, req *mcp.CallToolRequest, input blockContactInput) (*mcp.CallToolResult, sendResult, error) {
	if s.client == nil {
		return nil, sendResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	success, msg := s.client.BlockContact(input.JID)
	return nil, sendResult{Success: success, Message: msg}, nil
}

func (s *Server) handleUnblockContact(ctx context.Context, req *mcp.CallToolRequest, input unblockContactInput) (*mcp.CallToolResult, sendResult, error) {
	if s.client == nil {
		return nil, sendResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	success, msg := s.client.UnblockContact(input.JID)
	return nil, sendResult{Success: success, Message: msg}, nil
}

type blocklistResult struct {
	BlockedJIDs []string `json:"blocked_jids"`
	Count       int      `json:"count"`
}

func (s *Server) handleGetBlocklist(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, blocklistResult, error) {
	if s.client == nil {
		return nil, blocklistResult{}, fmt.Errorf("WhatsApp client not available")
	}
	jids, err := s.client.GetBlocklist()
	if err != nil {
		return nil, blocklistResult{}, err
	}
	if jids == nil {
		jids = []string{}
	}
	return nil, blocklistResult{BlockedJIDs: jids, Count: len(jids)}, nil
}

func (s *Server) handleMuteChat(ctx context.Context, req *mcp.CallToolRequest, input muteChatInput) (*mcp.CallToolResult, sendResult, error) {
	if s.client == nil {
		return nil, sendResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	if !input.Mute {
		success, msg := s.client.UnmuteChat(input.ChatJID)
		return nil, sendResult{Success: success, Message: msg}, nil
	}
	duration := time.Duration(input.DurationHours) * time.Hour
	success, msg := s.client.MuteChat(input.ChatJID, duration)
	return nil, sendResult{Success: success, Message: msg}, nil
}

func (s *Server) handlePinChat(ctx context.Context, req *mcp.CallToolRequest, input pinChatInput) (*mcp.CallToolResult, sendResult, error) {
	if s.client == nil {
		return nil, sendResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	success, msg := s.client.PinChat(input.ChatJID, input.Pin)
	return nil, sendResult{Success: success, Message: msg}, nil
}

func (s *Server) handleArchiveChat(ctx context.Context, req *mcp.CallToolRequest, input archiveChatInput) (*mcp.CallToolResult, sendResult, error) {
	if s.client == nil {
		return nil, sendResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	success, msg := s.client.ArchiveChat(input.ChatJID, input.Archive)
	return nil, sendResult{Success: success, Message: msg}, nil
}

func (s *Server) handleDeleteChat(ctx context.Context, req *mcp.CallToolRequest, input deleteChatInput) (*mcp.CallToolResult, sendResult, error) {
	if s.client == nil {
		return nil, sendResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	success, msg := s.client.DeleteChat(input.ChatJID)
	return nil, sendResult{Success: success, Message: msg}, nil
}

func (s *Server) handleMarkChatRead(ctx context.Context, req *mcp.CallToolRequest, input markChatReadInput) (*mcp.CallToolResult, sendResult, error) {
	if s.client == nil {
		return nil, sendResult{Success: false, Message: "WhatsApp client not available"}, nil
	}
	success, msg := s.client.MarkChatAsRead(input.ChatJID, input.Read)
	return nil, sendResult{Success: success, Message: msg}, nil
}

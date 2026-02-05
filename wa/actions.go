package wa

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// RevokeMessage deletes/revokes a message.
// For own messages: pass empty senderJID.
// For others' messages (as group admin): pass the original sender's JID.
func (c *Client) RevokeMessage(chatJID, messageID, senderJID string) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return false, fmt.Sprintf("Invalid chat JID: %v", err)
	}

	var sender types.JID
	if senderJID != "" {
		sender, err = types.ParseJID(senderJID)
		if err != nil {
			return false, fmt.Sprintf("Invalid sender JID: %v", err)
		}
	}

	revokeMsg := c.WA.BuildRevoke(chat, sender, messageID)
	_, err = c.WA.SendMessage(context.Background(), chat, revokeMsg)
	if err != nil {
		return false, fmt.Sprintf("Failed to revoke message: %v", err)
	}

	return true, fmt.Sprintf("Message %s revoked in %s", messageID, chatJID)
}

// BlockContact adds a contact to the blocklist.
func (c *Client) BlockContact(jidStr string) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return false, fmt.Sprintf("Invalid JID: %v", err)
	}

	_, err = c.WA.UpdateBlocklist(context.Background(), jid, "block")
	if err != nil {
		return false, fmt.Sprintf("Failed to block contact: %v", err)
	}

	return true, fmt.Sprintf("Contact %s blocked", jidStr)
}

// UnblockContact removes a contact from the blocklist.
func (c *Client) UnblockContact(jidStr string) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return false, fmt.Sprintf("Invalid JID: %v", err)
	}

	_, err = c.WA.UpdateBlocklist(context.Background(), jid, "unblock")
	if err != nil {
		return false, fmt.Sprintf("Failed to unblock contact: %v", err)
	}

	return true, fmt.Sprintf("Contact %s unblocked", jidStr)
}

// GetBlocklist returns the list of blocked contacts.
func (c *Client) GetBlocklist() ([]string, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to WhatsApp")
	}

	blocklist, err := c.WA.GetBlocklist(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get blocklist: %w", err)
	}

	var jids []string
	for _, jid := range blocklist.JIDs {
		jids = append(jids, jid.String())
	}
	return jids, nil
}

// MuteChat mutes a chat. duration=0 means mute forever.
func (c *Client) MuteChat(chatJID string, duration time.Duration) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return false, fmt.Sprintf("Invalid JID: %v", err)
	}

	err = c.WA.SendAppState(context.Background(), appstate.BuildMute(jid, true, duration))
	if err != nil {
		return false, fmt.Sprintf("Failed to mute chat: %v", err)
	}

	if duration == 0 {
		return true, fmt.Sprintf("Chat %s muted permanently", chatJID)
	}
	return true, fmt.Sprintf("Chat %s muted for %s", chatJID, duration)
}

// UnmuteChat unmutes a chat.
func (c *Client) UnmuteChat(chatJID string) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return false, fmt.Sprintf("Invalid JID: %v", err)
	}

	err = c.WA.SendAppState(context.Background(), appstate.BuildMute(jid, false, 0))
	if err != nil {
		return false, fmt.Sprintf("Failed to unmute chat: %v", err)
	}

	return true, fmt.Sprintf("Chat %s unmuted", chatJID)
}

// PinChat pins or unpins a chat.
func (c *Client) PinChat(chatJID string, pin bool) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return false, fmt.Sprintf("Invalid JID: %v", err)
	}

	err = c.WA.SendAppState(context.Background(), appstate.BuildPin(jid, pin))
	if err != nil {
		action := "pin"
		if !pin {
			action = "unpin"
		}
		return false, fmt.Sprintf("Failed to %s chat: %v", action, err)
	}

	if pin {
		return true, fmt.Sprintf("Chat %s pinned", chatJID)
	}
	return true, fmt.Sprintf("Chat %s unpinned", chatJID)
}

// ArchiveChat archives or unarchives a chat.
func (c *Client) ArchiveChat(chatJID string, archive bool) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return false, fmt.Sprintf("Invalid JID: %v", err)
	}

	lastMsgTime, lastMsgKey := c.getLastMessageKey(chatJID)

	err = c.WA.SendAppState(context.Background(), appstate.BuildArchive(jid, archive, lastMsgTime, lastMsgKey))
	if err != nil {
		action := "archive"
		if !archive {
			action = "unarchive"
		}
		return false, fmt.Sprintf("Failed to %s chat: %v", action, err)
	}

	if archive {
		return true, fmt.Sprintf("Chat %s archived", chatJID)
	}
	return true, fmt.Sprintf("Chat %s unarchived", chatJID)
}

// DeleteChat deletes a chat entirely.
func (c *Client) DeleteChat(chatJID string) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return false, fmt.Sprintf("Invalid JID: %v", err)
	}

	lastMsgTime, lastMsgKey := c.getLastMessageKey(chatJID)

	err = c.WA.SendAppState(context.Background(), appstate.BuildDeleteChat(jid, lastMsgTime, lastMsgKey, true))
	if err != nil {
		return false, fmt.Sprintf("Failed to delete chat: %v", err)
	}

	// Also remove from local DB (ignore errors - best effort cleanup)
	_, _ = c.Store.MsgDB.Exec("DELETE FROM messages WHERE chat_jid = ?", chatJID)
	_, _ = c.Store.MsgDB.Exec("DELETE FROM chats WHERE jid = ?", chatJID)

	return true, fmt.Sprintf("Chat %s deleted", chatJID)
}

// MarkChatAsRead marks a chat as read or unread.
func (c *Client) MarkChatAsRead(chatJID string, read bool) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return false, fmt.Sprintf("Invalid JID: %v", err)
	}

	_, lastMsgKey := c.getLastMessageKey(chatJID)

	err = c.WA.SendAppState(context.Background(), appstate.BuildMarkChatAsRead(jid, read, time.Now(), lastMsgKey))
	if err != nil {
		action := "read"
		if !read {
			action = "unread"
		}
		return false, fmt.Sprintf("Failed to mark as %s: %v", action, err)
	}

	if read {
		return true, fmt.Sprintf("Chat %s marked as read", chatJID)
	}
	return true, fmt.Sprintf("Chat %s marked as unread", chatJID)
}

// getLastMessageKey retrieves the last message's timestamp and key for a chat.
func (c *Client) getLastMessageKey(chatJID string) (time.Time, *waCommon.MessageKey) {
	var lastMsgID, lastSender string
	var lastMsgTime time.Time
	var isFromMe bool

	err := c.Store.MsgDB.QueryRow(
		"SELECT id, sender, timestamp, is_from_me FROM messages WHERE chat_jid = ? ORDER BY timestamp DESC LIMIT 1",
		chatJID,
	).Scan(&lastMsgID, &lastSender, &lastMsgTime, &isFromMe)

	if err != nil {
		return time.Now(), nil
	}

	key := &waCommon.MessageKey{
		RemoteJID: proto.String(chatJID),
		ID:        proto.String(lastMsgID),
		FromMe:    proto.Bool(isFromMe),
	}
	if !isFromMe && lastSender != "" {
		key.Participant = proto.String(lastSender)
	}

	return lastMsgTime, key
}

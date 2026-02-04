package wa

import (
	"context"
	"fmt"
	"reflect"

	"go.mau.fi/whatsmeow/types"
)

// GetChatName determines the display name for a chat.
// conversation is optional and used during history sync (may be *waProto.Conversation).
func GetChatName(c *Client, jid types.JID, chatJID string, conversation interface{}, sender string) string {
	// Check if chat already has a name in DB
	var existingName string
	err := c.Store.MsgDB.QueryRow("SELECT name FROM chats WHERE jid = ?", chatJID).Scan(&existingName)
	if err == nil && existingName != "" {
		return existingName
	}

	var name string

	if jid.Server == "g.us" {
		// Group chat
		if conversation != nil {
			name = extractConversationName(conversation)
		}

		if name == "" {
			groupInfo, err := c.WA.GetGroupInfo(context.Background(), jid)
			if err == nil && groupInfo.Name != "" {
				name = groupInfo.Name
			} else {
				name = fmt.Sprintf("Group %s", jid.User)
			}
		}
	} else {
		// Individual contact
		contact, err := c.WA.Store.Contacts.GetContact(context.Background(), jid)
		if err == nil && contact.FullName != "" {
			name = contact.FullName
		} else if sender != "" {
			name = sender
		} else {
			name = jid.User
		}
	}

	return name
}

// extractConversationName uses reflection to get DisplayName or Name from a conversation object.
func extractConversationName(conversation interface{}) string {
	v := reflect.ValueOf(conversation)
	if v.Kind() == reflect.Ptr && !v.IsNil() {
		v = v.Elem()

		if f := v.FieldByName("DisplayName"); f.IsValid() && f.Kind() == reflect.Ptr && !f.IsNil() {
			if dn := f.Elem().String(); dn != "" {
				return dn
			}
		}
		if f := v.FieldByName("Name"); f.IsValid() && f.Kind() == reflect.Ptr && !f.IsNil() {
			if n := f.Elem().String(); n != "" {
				return n
			}
		}
	}
	return ""
}

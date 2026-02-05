package wa

import (
	"fmt"
	"os"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// extractTextContent extracts text from a WhatsApp message proto.
func extractTextContent(msg *waProto.Message) string {
	if msg == nil {
		return ""
	}
	if text := msg.GetConversation(); text != "" {
		return text
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	return ""
}

// extractMediaInfo extracts media metadata from a WhatsApp message proto.
func extractMediaInfo(msg *waProto.Message) (mediaType, filename, url string, mediaKey, fileSHA256, fileEncSHA256 []byte, fileLength uint64) {
	if msg == nil {
		return
	}

	if img := msg.GetImageMessage(); img != nil {
		return "image", "image_" + time.Now().Format("20060102_150405") + ".jpg",
			img.GetURL(), img.GetMediaKey(), img.GetFileSHA256(), img.GetFileEncSHA256(), img.GetFileLength()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return "video", "video_" + time.Now().Format("20060102_150405") + ".mp4",
			vid.GetURL(), vid.GetMediaKey(), vid.GetFileSHA256(), vid.GetFileEncSHA256(), vid.GetFileLength()
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		return "audio", "audio_" + time.Now().Format("20060102_150405") + ".ogg",
			aud.GetURL(), aud.GetMediaKey(), aud.GetFileSHA256(), aud.GetFileEncSHA256(), aud.GetFileLength()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		fn := doc.GetFileName()
		if fn == "" {
			fn = "document_" + time.Now().Format("20060102_150405")
		}
		return "document", fn,
			doc.GetURL(), doc.GetMediaKey(), doc.GetFileSHA256(), doc.GetFileEncSHA256(), doc.GetFileLength()
	}

	return
}

// handleMessage processes an incoming real-time message event.
func handleMessage(c *Client, msg *events.Message) {
	chatJID := msg.Info.Chat.String()
	sender := msg.Info.Sender.User

	name := GetChatName(c, msg.Info.Chat, chatJID, nil, sender)

	if err := c.Store.StoreChat(chatJID, name, msg.Info.Timestamp); err != nil {
		c.Logger.Warnf("Failed to store chat: %v", err)
	}

	content := extractTextContent(msg.Message)
	mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength := extractMediaInfo(msg.Message)

	if content == "" && mediaType == "" {
		return
	}

	err := c.Store.StoreMessage(
		msg.Info.ID, chatJID, sender, content, msg.Info.Timestamp, msg.Info.IsFromMe,
		mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength,
	)
	if err != nil {
		c.Logger.Warnf("Failed to store message: %v", err)
		return
	}

	// Log to stderr
	ts := msg.Info.Timestamp.Format("2006-01-02 15:04:05")
	dir := "←"
	if msg.Info.IsFromMe {
		dir = "→"
	}
	if mediaType != "" {
		fmt.Fprintf(os.Stderr, "[%s] %s %s: [%s: %s] %s\n", ts, dir, sender, mediaType, filename, content)
	} else {
		fmt.Fprintf(os.Stderr, "[%s] %s %s: %s\n", ts, dir, sender, content)
	}
}

// handleHistorySync processes a history sync event.
func handleHistorySync(c *Client, historySync *events.HistorySync) {
	fmt.Fprintf(os.Stderr, "History sync: %d conversations\n", len(historySync.Data.Conversations))

	syncedCount := 0
	for _, conversation := range historySync.Data.Conversations {
		if conversation.ID == nil {
			continue
		}
		chatJID := *conversation.ID

		jid, err := types.ParseJID(chatJID)
		if err != nil {
			c.Logger.Warnf("Failed to parse JID %s: %v", chatJID, err)
			continue
		}

		name := GetChatName(c, jid, chatJID, conversation, "")

		messages := conversation.Messages
		if len(messages) == 0 {
			continue
		}

		// Update chat with latest message timestamp
		latestMsg := messages[0]
		if latestMsg == nil || latestMsg.Message == nil {
			continue
		}

		ts := latestMsg.Message.GetMessageTimestamp()
		if ts == 0 {
			continue
		}
		timestamp := time.Unix(int64(ts), 0)
		c.Store.StoreChat(chatJID, name, timestamp)

		// Store messages
		for _, msg := range messages {
			if msg == nil || msg.Message == nil {
				continue
			}

			content := extractTextContent(msg.Message.Message)
			mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength := extractMediaInfo(msg.Message.Message)

			if content == "" && mediaType == "" {
				continue
			}

			// Determine sender
			var sender string
			isFromMe := false
			if msg.Message.Key != nil {
				if msg.Message.Key.FromMe != nil {
					isFromMe = *msg.Message.Key.FromMe
				}
				if !isFromMe && msg.Message.Key.Participant != nil && *msg.Message.Key.Participant != "" {
					sender = *msg.Message.Key.Participant
				} else if isFromMe {
					sender = c.WA.Store.ID.User
				} else {
					sender = jid.User
				}
			} else {
				sender = jid.User
			}

			msgID := ""
			if msg.Message.Key != nil && msg.Message.Key.ID != nil {
				msgID = *msg.Message.Key.ID
			}

			msgTs := msg.Message.GetMessageTimestamp()
			if msgTs == 0 {
				continue
			}
			msgTime := time.Unix(int64(msgTs), 0)

			err = c.Store.StoreMessage(
				msgID, chatJID, sender, content, msgTime, isFromMe,
				mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength,
			)
			if err != nil {
				c.Logger.Warnf("Failed to store history message: %v", err)
			} else {
				syncedCount++
			}
		}
	}

	fmt.Fprintf(os.Stderr, "History sync complete. Stored %d messages.\n", syncedCount)
}

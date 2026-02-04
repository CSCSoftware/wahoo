package wa

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// SendMessage sends a text message to a recipient.
func (c *Client) SendMessage(recipient, message string) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	jid, err := parseRecipient(recipient)
	if err != nil {
		return false, err.Error()
	}

	msg := &waProto.Message{
		Conversation: proto.String(message),
	}

	_, err = c.WA.SendMessage(context.Background(), jid, msg)
	if err != nil {
		return false, fmt.Sprintf("Error sending message: %v", err)
	}
	return true, fmt.Sprintf("Message sent to %s", recipient)
}

// SendMedia sends a file (image, video, document) to a recipient.
func (c *Client) SendMedia(recipient, mediaPath, caption string) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	jid, err := parseRecipient(recipient)
	if err != nil {
		return false, err.Error()
	}

	mediaData, err := os.ReadFile(mediaPath)
	if err != nil {
		return false, fmt.Sprintf("Error reading media file: %v", err)
	}

	fileExt := strings.ToLower(filepath.Ext(mediaPath))
	if fileExt != "" {
		fileExt = fileExt[1:] // remove dot
	}

	var mediaType whatsmeow.MediaType
	var mimeType string

	switch fileExt {
	case "jpg", "jpeg":
		mediaType, mimeType = whatsmeow.MediaImage, "image/jpeg"
	case "png":
		mediaType, mimeType = whatsmeow.MediaImage, "image/png"
	case "gif":
		mediaType, mimeType = whatsmeow.MediaImage, "image/gif"
	case "webp":
		mediaType, mimeType = whatsmeow.MediaImage, "image/webp"
	case "ogg":
		mediaType, mimeType = whatsmeow.MediaAudio, "audio/ogg; codecs=opus"
	case "mp4":
		mediaType, mimeType = whatsmeow.MediaVideo, "video/mp4"
	case "avi":
		mediaType, mimeType = whatsmeow.MediaVideo, "video/avi"
	case "mov":
		mediaType, mimeType = whatsmeow.MediaVideo, "video/quicktime"
	default:
		mediaType, mimeType = whatsmeow.MediaDocument, "application/octet-stream"
	}

	resp, err := c.WA.Upload(context.Background(), mediaData, mediaType)
	if err != nil {
		return false, fmt.Sprintf("Error uploading media: %v", err)
	}

	msg := &waProto.Message{}
	switch mediaType {
	case whatsmeow.MediaImage:
		msg.ImageMessage = &waProto.ImageMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mimeType),
			URL:           &resp.URL,
			DirectPath:    &resp.DirectPath,
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    &resp.FileLength,
		}
	case whatsmeow.MediaAudio:
		var seconds uint32 = 30
		var waveform []byte
		if strings.Contains(mimeType, "ogg") {
			if s, w, err := analyzeOggOpus(mediaData); err == nil {
				seconds, waveform = s, w
			}
		}
		msg.AudioMessage = &waProto.AudioMessage{
			Mimetype:      proto.String(mimeType),
			URL:           &resp.URL,
			DirectPath:    &resp.DirectPath,
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    &resp.FileLength,
			Seconds:       proto.Uint32(seconds),
			PTT:           proto.Bool(true),
			Waveform:      waveform,
		}
	case whatsmeow.MediaVideo:
		msg.VideoMessage = &waProto.VideoMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mimeType),
			URL:           &resp.URL,
			DirectPath:    &resp.DirectPath,
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    &resp.FileLength,
		}
	case whatsmeow.MediaDocument:
		msg.DocumentMessage = &waProto.DocumentMessage{
			Title:         proto.String(filepath.Base(mediaPath)),
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mimeType),
			URL:           &resp.URL,
			DirectPath:    &resp.DirectPath,
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    &resp.FileLength,
		}
	}

	_, err = c.WA.SendMessage(context.Background(), jid, msg)
	if err != nil {
		return false, fmt.Sprintf("Error sending media: %v", err)
	}
	return true, fmt.Sprintf("Media sent to %s", recipient)
}

// SendAudioMessage sends an audio file as a voice message, converting to OGG Opus if needed.
func (c *Client) SendAudioMessage(recipient, mediaPath string) (bool, string) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	// Convert to OGG Opus if not already
	if !strings.HasSuffix(strings.ToLower(mediaPath), ".ogg") {
		converted, err := convertToOpusOgg(mediaPath)
		if err != nil {
			return false, fmt.Sprintf("Error converting to Opus OGG (ffmpeg needed): %v", err)
		}
		mediaPath = converted
		defer os.Remove(converted)
	}

	return c.SendMedia(recipient, mediaPath, "")
}

// DownloadMedia downloads media from a message and saves it to disk.
func (c *Client) DownloadMedia(messageID, chatJID string) (string, error) {
	if !c.IsConnected() {
		return "", fmt.Errorf("not connected to WhatsApp")
	}

	url, mediaKey, fileSHA256, fileEncSHA256, fileLength, mediaType, filename, err := c.Store.GetMediaInfo(messageID, chatJID)
	if err != nil {
		return "", fmt.Errorf("failed to find message: %w", err)
	}

	if mediaType == "" {
		return "", fmt.Errorf("not a media message")
	}

	// Create download directory
	chatDir := filepath.Join(c.StoreDir, strings.ReplaceAll(chatJID, ":", "_"))
	if err := os.MkdirAll(chatDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	localPath := filepath.Join(chatDir, filename)
	absPath, _ := filepath.Abs(localPath)

	// Check if already downloaded
	if _, err := os.Stat(localPath); err == nil {
		return absPath, nil
	}

	// Need all media info to download
	if url == "" || len(mediaKey) == 0 {
		return "", fmt.Errorf("incomplete media information")
	}

	// Map media type string to whatsmeow type
	var waMediaType whatsmeow.MediaType
	switch mediaType {
	case "image":
		waMediaType = whatsmeow.MediaImage
	case "video":
		waMediaType = whatsmeow.MediaVideo
	case "audio":
		waMediaType = whatsmeow.MediaAudio
	case "document":
		waMediaType = whatsmeow.MediaDocument
	default:
		return "", fmt.Errorf("unsupported media type: %s", mediaType)
	}

	directPath := extractDirectPathFromURL(url)

	downloader := &MediaDownloader{
		URL:           url,
		DirectPath:    directPath,
		MediaKey:      mediaKey,
		FileLength:    fileLength,
		FileSHA256:    fileSHA256,
		FileEncSHA256: fileEncSHA256,
		MediaType:     waMediaType,
	}

	data, err := c.WA.Download(context.Background(), downloader)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	if err := os.WriteFile(localPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to save file: %w", err)
	}

	return absPath, nil
}

// MediaDownloader implements whatsmeow.DownloadableMessage.
type MediaDownloader struct {
	URL           string
	DirectPath    string
	MediaKey      []byte
	FileLength    uint64
	FileSHA256    []byte
	FileEncSHA256 []byte
	MediaType     whatsmeow.MediaType
}

func (d *MediaDownloader) GetDirectPath() string         { return d.DirectPath }
func (d *MediaDownloader) GetURL() string                 { return d.URL }
func (d *MediaDownloader) GetMediaKey() []byte            { return d.MediaKey }
func (d *MediaDownloader) GetFileLength() uint64          { return d.FileLength }
func (d *MediaDownloader) GetFileSHA256() []byte          { return d.FileSHA256 }
func (d *MediaDownloader) GetFileEncSHA256() []byte       { return d.FileEncSHA256 }
func (d *MediaDownloader) GetMediaType() whatsmeow.MediaType { return d.MediaType }

// parseRecipient parses a phone number or JID string into a types.JID.
func parseRecipient(recipient string) (types.JID, error) {
	if strings.Contains(recipient, "@") {
		return types.ParseJID(recipient)
	}
	return types.JID{User: recipient, Server: "s.whatsapp.net"}, nil
}

// extractDirectPathFromURL extracts the direct path from a WhatsApp media URL.
func extractDirectPathFromURL(url string) string {
	parts := strings.SplitN(url, ".net/", 2)
	if len(parts) < 2 {
		return url
	}
	pathPart := strings.SplitN(parts[1], "?", 2)[0]
	return "/" + pathPart
}

// convertToOpusOgg converts any audio file to OGG Opus using ffmpeg.
func convertToOpusOgg(inputPath string) (string, error) {
	outPath := inputPath + ".ogg"
	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath,
		"-c:a", "libopus", "-b:a", "32k", "-vn", outPath)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg conversion failed: %w", err)
	}
	return outPath, nil
}

// analyzeOggOpus extracts duration and generates a waveform from an Ogg Opus file.
func analyzeOggOpus(data []byte) (duration uint32, waveform []byte, err error) {
	if len(data) < 4 || string(data[0:4]) != "OggS" {
		return 0, nil, fmt.Errorf("not a valid Ogg file")
	}

	var lastGranule uint64
	var sampleRate uint32 = 48000
	var preSkip uint16
	var foundOpusHead bool

	for i := 0; i < len(data); {
		if i+27 >= len(data) {
			break
		}
		if string(data[i:i+4]) != "OggS" {
			i++
			continue
		}

		granulePos := binary.LittleEndian.Uint64(data[i+6 : i+14])
		pageSeqNum := binary.LittleEndian.Uint32(data[i+18 : i+22])
		numSegments := int(data[i+26])

		if i+27+numSegments >= len(data) {
			break
		}
		segmentTable := data[i+27 : i+27+numSegments]

		pageSize := 27 + numSegments
		for _, segLen := range segmentTable {
			pageSize += int(segLen)
		}

		if !foundOpusHead && pageSeqNum <= 1 {
			pageData := data[i : i+pageSize]
			headPos := bytes.Index(pageData, []byte("OpusHead"))
			if headPos >= 0 && headPos+16 <= len(pageData) {
				headPos += 8
				if headPos+8 <= len(pageData) {
					preSkip = binary.LittleEndian.Uint16(pageData[headPos+2 : headPos+4])
					sampleRate = binary.LittleEndian.Uint32(pageData[headPos+4 : headPos+8])
					foundOpusHead = true
				}
			}
		}

		if granulePos != 0 {
			lastGranule = granulePos
		}
		i += pageSize
	}

	if lastGranule > 0 {
		durationSeconds := float64(lastGranule-uint64(preSkip)) / float64(sampleRate)
		duration = uint32(math.Ceil(durationSeconds))
	} else {
		duration = uint32(float64(len(data)) / 2000.0)
	}

	if duration < 1 {
		duration = 1
	} else if duration > 300 {
		duration = 300
	}

	waveform = placeholderWaveform(duration)
	return duration, waveform, nil
}

// placeholderWaveform generates a synthetic waveform for voice messages.
func placeholderWaveform(duration uint32) []byte {
	const waveformLength = 64
	waveform := make([]byte, waveformLength)

	r := rand.New(rand.NewSource(int64(duration)))
	baseAmplitude := 35.0
	freq := math.Min(float64(duration), 120) / 30.0

	for i := range waveform {
		pos := float64(i) / float64(waveformLength)
		val := baseAmplitude * math.Sin(pos*math.Pi*freq*8)
		val += (baseAmplitude / 2) * math.Sin(pos*math.Pi*freq*16)
		val += (r.Float64() - 0.5) * 15
		val *= 0.7 + 0.3*math.Sin(pos*math.Pi)
		val += 50

		if val < 0 {
			val = 0
		} else if val > 100 {
			val = 100
		}
		waveform[i] = byte(val)
	}
	return waveform
}

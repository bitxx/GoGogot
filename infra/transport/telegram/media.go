package telegram

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gogogot/infra/transport"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	maxTextFileSize    = 512 * 1024       // 512 KB
	maxImageFileSize   = 10 * 1024 * 1024 // 10 MB
	maxGenericFileSize = 20 * 1024 * 1024 // 20 MB
)

func (t *Transport) downloadFile(fileID string) ([]byte, error) {
	file, err := t.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("get file info: %w", err)
	}
	url := file.Link(t.api.Token)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func isTextMIME(mime string) bool {
	if strings.HasPrefix(mime, "text/") {
		return true
	}
	textTypes := []string{
		"application/json", "application/xml", "application/javascript",
		"application/x-yaml", "application/toml", "application/x-sh",
		"application/csv", "application/sql",
	}
	for _, t := range textTypes {
		if mime == t {
			return true
		}
	}
	return false
}

func (t *Transport) processDocument(doc *tgbotapi.Document) ([]transport.Attachment, error) {
	mime := doc.MimeType

	if mime == "application/zip" || mime == "application/x-zip-compressed" || strings.HasSuffix(strings.ToLower(doc.FileName), ".zip") {
		if doc.FileSize > 20*1024*1024 {
			return nil, fmt.Errorf("zip file too large (%d bytes)", doc.FileSize)
		}
		data, err := t.downloadFile(doc.FileID)
		if err != nil {
			return nil, err
		}
		return extractZipFiles(data)
	}

	if mime == "application/gzip" || mime == "application/x-gzip" || strings.HasSuffix(strings.ToLower(doc.FileName), ".tar.gz") || strings.HasSuffix(strings.ToLower(doc.FileName), ".tgz") {
		if doc.FileSize > 20*1024*1024 {
			return nil, fmt.Errorf("tar.gz file too large (%d bytes)", doc.FileSize)
		}
		data, err := t.downloadFile(doc.FileID)
		if err != nil {
			return nil, err
		}
		return extractTarGzFiles(data)
	}

	if strings.HasPrefix(mime, "image/") {
		if doc.FileSize > maxImageFileSize {
			return nil, fmt.Errorf("image too large (%d bytes)", doc.FileSize)
		}
		data, err := t.downloadFile(doc.FileID)
		if err != nil {
			return nil, err
		}
		return []transport.Attachment{{
			Filename: doc.FileName,
			MimeType: mime,
			Data:     data,
		}}, nil
	}

	if isTextMIME(mime) || mime == "" || mime == "application/octet-stream" {
		if doc.FileSize > maxTextFileSize {
			return nil, fmt.Errorf("text file too large (%d bytes)", doc.FileSize)
		}
		data, err := t.downloadFile(doc.FileID)
		if err != nil {
			return nil, err
		}
		return []transport.Attachment{{
			Filename: doc.FileName,
			MimeType: "text/plain",
			Data:     data,
		}}, nil
	}

	if doc.FileSize > maxGenericFileSize {
		return nil, fmt.Errorf("file too large (%d bytes, max %d)", doc.FileSize, maxGenericFileSize)
	}
	data, err := t.downloadFile(doc.FileID)
	if err != nil {
		return nil, err
	}
	return []transport.Attachment{{
		Filename: doc.FileName,
		MimeType: mime,
		Data:     data,
	}}, nil
}

func (t *Transport) processPhoto(photos []tgbotapi.PhotoSize) (*transport.Attachment, error) {
	if len(photos) == 0 {
		return nil, nil
	}
	largest := photos[len(photos)-1]
	if largest.FileSize > maxImageFileSize {
		return nil, fmt.Errorf("photo too large (%d bytes)", largest.FileSize)
	}
	data, err := t.downloadFile(largest.FileID)
	if err != nil {
		return nil, err
	}
	return &transport.Attachment{
		Filename: "photo.jpg",
		MimeType: "image/jpeg",
		Data:     data,
	}, nil
}

func (t *Transport) processAudio(audio *tgbotapi.Audio) (*transport.Attachment, error) {
	if audio.FileSize > maxGenericFileSize {
		return nil, fmt.Errorf("audio too large (%d bytes)", audio.FileSize)
	}
	data, err := t.downloadFile(audio.FileID)
	if err != nil {
		return nil, err
	}
	filename := audio.FileName
	if filename == "" {
		filename = "audio.mp3"
	}
	mime := audio.MimeType
	if mime == "" {
		mime = "audio/mpeg"
	}
	return &transport.Attachment{Filename: filename, MimeType: mime, Data: data}, nil
}

func (t *Transport) processVoice(voice *tgbotapi.Voice) (*transport.Attachment, error) {
	if voice.FileSize > maxGenericFileSize {
		return nil, fmt.Errorf("voice too large (%d bytes)", voice.FileSize)
	}
	data, err := t.downloadFile(voice.FileID)
	if err != nil {
		return nil, err
	}
	mime := voice.MimeType
	if mime == "" {
		mime = "audio/ogg"
	}
	return &transport.Attachment{Filename: "voice.ogg", MimeType: mime, Data: data}, nil
}

func (t *Transport) processVideo(video *tgbotapi.Video) (*transport.Attachment, error) {
	if video.FileSize > maxGenericFileSize {
		return nil, fmt.Errorf("video too large (%d bytes)", video.FileSize)
	}
	data, err := t.downloadFile(video.FileID)
	if err != nil {
		return nil, err
	}
	mime := video.MimeType
	if mime == "" {
		mime = "video/mp4"
	}
	filename := video.FileName
	if filename == "" {
		filename = "video.mp4"
	}
	return &transport.Attachment{Filename: filename, MimeType: mime, Data: data}, nil
}

func (t *Transport) processVideoNote(vn *tgbotapi.VideoNote) (*transport.Attachment, error) {
	if vn.FileSize > maxGenericFileSize {
		return nil, fmt.Errorf("video note too large (%d bytes)", vn.FileSize)
	}
	data, err := t.downloadFile(vn.FileID)
	if err != nil {
		return nil, err
	}
	return &transport.Attachment{Filename: "videonote.mp4", MimeType: "video/mp4", Data: data}, nil
}

func (t *Transport) processAnimation(anim *tgbotapi.Animation) (*transport.Attachment, error) {
	if anim.FileSize > maxGenericFileSize {
		return nil, fmt.Errorf("animation too large (%d bytes)", anim.FileSize)
	}
	data, err := t.downloadFile(anim.FileID)
	if err != nil {
		return nil, err
	}
	mime := anim.MimeType
	if mime == "" {
		mime = "video/mp4"
	}
	filename := anim.FileName
	if filename == "" {
		filename = "animation.mp4"
	}
	return &transport.Attachment{Filename: filename, MimeType: mime, Data: data}, nil
}

func (t *Transport) processSticker(sticker *tgbotapi.Sticker) (*transport.Attachment, error) {
	if sticker.IsAnimated {
		return nil, nil
	}
	if sticker.FileSize > maxImageFileSize {
		return nil, fmt.Errorf("sticker too large (%d bytes)", sticker.FileSize)
	}
	data, err := t.downloadFile(sticker.FileID)
	if err != nil {
		return nil, err
	}
	return &transport.Attachment{Filename: "sticker.webp", MimeType: "image/webp", Data: data}, nil
}

func extractZipFiles(data []byte) ([]transport.Attachment, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to read zip: %w", err)
	}

	var attachments []transport.Attachment
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if strings.HasPrefix(file.Name, ".") || strings.Contains(file.Name, "/.") || strings.Contains(file.Name, "__MACOSX") {
			continue
		}

		ext := strings.ToLower(file.Name)
		isText := isTextExtension(ext)
		isImage := isImageExtension(ext)

		if !isText && !isImage {
			continue
		}
		if file.UncompressedSize64 > maxTextFileSize && isText {
			continue
		}
		if file.UncompressedSize64 > maxImageFileSize && isImage {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			continue
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		attachments = append(attachments, transport.Attachment{
			Filename: file.Name,
			MimeType: mimeFromExtension(ext, isImage),
			Data:     content,
		})

		if len(attachments) >= 20 {
			break
		}
	}

	return attachments, nil
}

func extractTarGzFiles(data []byte) ([]transport.Attachment, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to read gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var attachments []transport.Attachment

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}
		if strings.HasPrefix(header.Name, ".") || strings.Contains(header.Name, "/.") || strings.Contains(header.Name, "__MACOSX") {
			continue
		}

		ext := strings.ToLower(header.Name)
		isText := isTextExtension(ext)
		isImage := isImageExtension(ext)

		if !isText && !isImage {
			continue
		}
		if header.Size > maxTextFileSize && isText {
			continue
		}
		if header.Size > maxImageFileSize && isImage {
			continue
		}

		content, err := io.ReadAll(tr)
		if err != nil {
			continue
		}

		attachments = append(attachments, transport.Attachment{
			Filename: header.Name,
			MimeType: mimeFromExtension(ext, isImage),
			Data:     content,
		})

		if len(attachments) >= 20 {
			break
		}
	}

	return attachments, nil
}

func isTextExtension(name string) bool {
	textExts := []string{".txt", ".md", ".json", ".go", ".py", ".js", ".html", ".css", ".csv", ".yaml", ".yml", ".xml"}
	for _, e := range textExts {
		if strings.HasSuffix(name, e) {
			return true
		}
	}
	return false
}

func isImageExtension(name string) bool {
	imageExts := []string{".png", ".jpg", ".jpeg", ".webp"}
	for _, e := range imageExts {
		if strings.HasSuffix(name, e) {
			return true
		}
	}
	return false
}

func mimeFromExtension(name string, isImage bool) string {
	if !isImage {
		return "text/plain"
	}
	if strings.HasSuffix(name, ".png") {
		return "image/png"
	}
	if strings.HasSuffix(name, ".webp") {
		return "image/webp"
	}
	return "image/jpeg"
}

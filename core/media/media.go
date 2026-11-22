// Package media classifies files by extension into preview categories.
package media

import (
	"mime"
	"path/filepath"
	"strings"
)

var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".bmp": true, ".ico": true, ".avif": true,
}

var videoExtensions = map[string]bool{
	".mp4": true, ".webm": true, ".ogv": true, ".mov": true, ".m4v": true, ".mkv": true,
}

var audioExtensions = map[string]bool{
	".mp3": true, ".wav": true, ".ogg": true, ".flac": true, ".aac": true,
	".m4a": true, ".opus": true,
}

var textExtensions = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".json": true, ".yaml": true,
	".yml": true, ".toml": true, ".csv": true, ".xml": true, ".js": true,
	".ts": true, ".css": true, ".go": true, ".py": true, ".sh": true,
	".bash": true, ".rs": true, ".c": true, ".cpp": true, ".h": true,
	".java": true, ".rb": true, ".php": true, ".sql": true, ".ini": true,
	".conf": true, ".log": true,
}

// PreviewType returns the media type category based on the file extension.
func PreviewType(filename string) string {
	lowerFilename := strings.ToLower(filename)

	index := strings.LastIndex(lowerFilename, ".")
	if index < 0 {
		return "unknown"
	}

	extension := lowerFilename[index:]

	switch {
	case imageExtensions[extension]:
		return "image"
	case videoExtensions[extension]:
		return "video"
	case audioExtensions[extension]:
		return "audio"
	case textExtensions[extension]:
		return "text"
	default:
		return "unknown"
	}
}

// ContentType returns the MIME type for the given filename's extension. It corrects .webm, which the mime package resolves
// to audio/webm on several platforms, to video/webm so that embed consumers recognize the response as a playable video.
func ContentType(filename string) string {
	extension := strings.ToLower(filepath.Ext(filename))
	if extension == "" {
		return ""
	}

	if extension == ".webm" {
		return "video/webm"
	}

	return mime.TypeByExtension(extension)
}

// InlineContentType returns the Content-Type to use when serving a file inline, and whether the file is safe to render inline.
func InlineContentType(filename string) (string, bool) {
	switch PreviewType(filename) {
	case "image", "video", "audio":
		contentType := ContentType(filename)
		if contentType == "" {
			return "application/octet-stream", false
		}

		return contentType, true
	case "text":
		return "text/plain; charset=utf-8", true
	default:
		return "application/octet-stream", false
	}
}

// IsInlineContentType reports whether a MIME type is safe to serve inline.
func IsInlineContentType(contentType string) bool {
	switch {
	case strings.HasPrefix(contentType, "image/svg+xml"):
		return false
	case strings.HasPrefix(contentType, "image/"),
		strings.HasPrefix(contentType, "video/"),
		strings.HasPrefix(contentType, "audio/"),
		contentType == "text/plain",
		strings.HasPrefix(contentType, "text/plain;"):
		return true
	default:
		return false
	}
}

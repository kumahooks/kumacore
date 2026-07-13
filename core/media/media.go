// Package media classifies files by extension into preview categories.
package media

import "strings"

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

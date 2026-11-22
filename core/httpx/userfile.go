package httpx

import (
	"fmt"
	"mime"
	"net/http"
	"os"

	"kumacore/core/media"
)

// UserFile describes a user-supplied file that is streamed to the client.
//
// ContentType is optional. When set it is honored only if media considers it safe inline. Otherwise the response is
// forced to application/octet-stream. When empty, the content type is derived from Filename.
type UserFile struct {
	Path        string
	Filename    string
	ContentType string
	Attachment  bool
}

// ServeUserFile streams a user-supplied file with the shared inline-content policy.
func ServeUserFile(writer http.ResponseWriter, request *http.Request, userFile UserFile) error {
	file, err := os.Open(userFile.Path)
	if err != nil {
		return fmt.Errorf("[httpx:ServeUserFile] open: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("[httpx:ServeUserFile] stat: %w", err)
	}

	contentType, safeInline := media.InlineContentType(userFile.Filename)
	if userFile.ContentType != "" {
		if media.IsInlineContentType(userFile.ContentType) {
			contentType = userFile.ContentType
			safeInline = true
		} else {
			contentType = "application/octet-stream"
			safeInline = false
		}
	}

	disposition := "attachment"
	if !userFile.Attachment && safeInline {
		disposition = "inline"
	}

	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	writer.Header().Set(
		"Content-Disposition",
		mime.FormatMediaType(disposition, map[string]string{"filename": userFile.Filename}),
	)

	http.ServeContent(writer, request, userFile.Filename, fileInfo.ModTime(), file)

	return nil
}

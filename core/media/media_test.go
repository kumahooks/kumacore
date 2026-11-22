package media_test

import (
	"testing"

	"kumacore/core/media"
)

func TestPreviewType(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		want     string
	}{
		{"jpg", "photo.jpg", "image"},
		{"jpeg", "photo.jpeg", "image"},
		{"png", "img.png", "image"},
		{"gif", "anim.gif", "image"},
		{"webp", "photo.webp", "image"},
		{"bmp", "photo.bmp", "image"},
		{"ico", "favicon.ico", "image"},
		{"avif", "photo.avif", "image"},

		{"mp4", "clip.mp4", "video"},
		{"webm", "clip.webm", "video"},
		{"ogv", "clip.ogv", "video"},
		{"mov", "clip.mov", "video"},
		{"m4v", "clip.m4v", "video"},
		{"mkv", "clip.mkv", "video"},

		{"mp3", "song.mp3", "audio"},
		{"wav", "song.wav", "audio"},
		{"ogg", "song.ogg", "audio"},
		{"flac", "song.flac", "audio"},
		{"aac", "song.aac", "audio"},
		{"m4a", "song.m4a", "audio"},
		{"opus", "song.opus", "audio"},

		{"txt", "notes.txt", "text"},
		{"md", "README.md", "text"},
		{"go", "main.go", "text"},
		{"json", "data.json", "text"},
		{"yaml", "conf.yaml", "text"},
		{"sql", "query.sql", "text"},

		{"unknown extension", "file.xyz", "unknown"},
		{"no extension", "README", "unknown"},
		{"double extension takes last", "archive.tar.gz", "unknown"},

		{"uppercase jpg", "PHOTO.JPG", "image"},
		{"mixed case png", "Image.PnG", "image"},
		{"uppercase mp4", "CLIP.MP4", "video"},
		{"uppercase go", "MAIN.GO", "text"},

		{"extension only jpg", ".jpg", "image"},
		{"extension only mp4", ".mp4", "video"},
		{"extension only unknown", ".xyz", "unknown"},

		{"empty filename", "", "unknown"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := media.PreviewType(testCase.filename)
			if got != testCase.want {
				t.Errorf("PreviewType(%q) = %q, want %q", testCase.filename, got, testCase.want)
			}
		})
	}
}

func TestContentType(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		want     string
	}{
		{"webm corrected to video", "clip.webm", "video/webm"},
		{"webm uppercase corrected", "CLIP.WEBM", "video/webm"},
		{"png", "img.png", "image/png"},
		{"jpg", "photo.jpg", "image/jpeg"},
		{"mp4", "clip.mp4", "video/mp4"},
		{"json", "data.json", "application/json"},
		{"html", "page.html", "text/html; charset=utf-8"},
		{"css", "style.css", "text/css; charset=utf-8"},
		{"no extension returns empty", "README", ""},
		{"unknown extension returns empty", "file.zzzzz", ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := media.ContentType(testCase.filename)
			if got != testCase.want {
				t.Errorf("ContentType(%q) = %q, want %q", testCase.filename, got, testCase.want)
			}
		})
	}
}

func TestInlineContentType(t *testing.T) {
	cases := []struct {
		name            string
		filename        string
		wantContentType string
		wantInline      bool
	}{
		{"jpeg inline", "photo.jpg", "image/jpeg", true},
		{"png inline", "img.png", "image/png", true},
		{"webm corrected inline", "clip.webm", "video/webm", true},
		{"mp3 inline", "song.mp3", "audio/mpeg", true},
		{"text forced to plain", "notes.txt", "text/plain; charset=utf-8", true},
		{"javascript forced to plain", "payload.js", "text/plain; charset=utf-8", true},
		{"xml forced to plain", "feed.xml", "text/plain; charset=utf-8", true},
		{"css forced to plain", "style.css", "text/plain; charset=utf-8", true},
		{"html never inline", "evil.html", "application/octet-stream", false},
		{"svg never inline", "evil.svg", "application/octet-stream", false},
		{"unknown never inline", "file.xyz", "application/octet-stream", false},
		{"no extension never inline", "README", "application/octet-stream", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			contentType, inline := media.InlineContentType(testCase.filename)
			if contentType != testCase.wantContentType {
				t.Errorf(
					"InlineContentType(%q) content type = %q, want %q",
					testCase.filename,
					contentType,
					testCase.wantContentType,
				)
			}

			if inline != testCase.wantInline {
				t.Errorf(
					"InlineContentType(%q) inline = %v, want %v",
					testCase.filename,
					inline,
					testCase.wantInline,
				)
			}
		})
	}
}

func TestIsInlineContentType(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		want        bool
	}{
		{"jpeg", "image/jpeg", true},
		{"png", "image/png", true},
		{"svg rejected", "image/svg+xml", false},
		{"video", "video/mp4", true},
		{"audio", "audio/mpeg", true},
		{"plain text", "text/plain; charset=utf-8", true},
		{"html rejected", "text/html; charset=utf-8", false},
		{"octet stream rejected", "application/octet-stream", false},
		{"json rejected", "application/json", false},
		{"empty rejected", "", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := media.IsInlineContentType(testCase.contentType); got != testCase.want {
				t.Errorf(
					"IsInlineContentType(%q) = %v, want %v",
					testCase.contentType,
					got,
					testCase.want,
				)
			}
		})
	}
}

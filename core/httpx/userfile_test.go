package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kumacore/core/httpx"
)

func writeUserFile(t *testing.T, name string, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	return path
}

func TestServeUserFile_SafeTypeInline(t *testing.T) {
	path := writeUserFile(t, "image.gif", "gif-bytes")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/file", nil)

	if err := httpx.ServeUserFile(recorder, request, httpx.UserFile{
		Path:     path,
		Filename: "image.gif",
	}); err != nil {
		t.Fatalf("serve user file: %v", err)
	}

	if got := recorder.Header().Get("Content-Type"); got != "image/gif" {
		t.Errorf("Content-Type: got %q, want image/gif", got)
	}

	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "inline") {
		t.Errorf("Content-Disposition: got %q, want inline", got)
	}

	if got := recorder.Header().Get("Content-Security-Policy"); got != "default-src 'none'; sandbox" {
		t.Errorf("Content-Security-Policy: got %q", got)
	}

	if got := recorder.Body.String(); got != "gif-bytes" {
		t.Errorf("body: got %q", got)
	}
}

func TestServeUserFile_ActiveTypeAttachment(t *testing.T) {
	path := writeUserFile(t, "evil.html", "<script>alert(1)</script>")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/file", nil)

	if err := httpx.ServeUserFile(recorder, request, httpx.UserFile{
		Path:     path,
		Filename: "evil.html",
	}); err != nil {
		t.Fatalf("serve user file: %v", err)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type: got %q, want application/octet-stream", got)
	}

	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Errorf("Content-Disposition: got %q, want attachment", got)
	}
}

func TestServeUserFile_StoredSafeContentTypeInline(t *testing.T) {
	path := writeUserFile(t, "attachment", "png-bytes")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/file", nil)

	if err := httpx.ServeUserFile(recorder, request, httpx.UserFile{
		Path:        path,
		Filename:    "attachment",
		ContentType: "image/png",
	}); err != nil {
		t.Fatalf("serve user file: %v", err)
	}

	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type: got %q, want image/png", got)
	}

	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "inline") {
		t.Errorf("Content-Disposition: got %q, want inline", got)
	}
}

func TestServeUserFile_StoredUnsafeContentTypeAttachment(t *testing.T) {
	path := writeUserFile(t, "attachment", "html-bytes")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/file", nil)

	if err := httpx.ServeUserFile(recorder, request, httpx.UserFile{
		Path:        path,
		Filename:    "attachment",
		ContentType: "text/html; charset=utf-8",
	}); err != nil {
		t.Fatalf("serve user file: %v", err)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type: got %q, want application/octet-stream", got)
	}

	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Errorf("Content-Disposition: got %q, want attachment", got)
	}
}

func TestServeUserFile_AttachmentOverridesSafeType(t *testing.T) {
	path := writeUserFile(t, "image.gif", "gif-bytes")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/file", nil)

	if err := httpx.ServeUserFile(recorder, request, httpx.UserFile{
		Path:       path,
		Filename:   "image.gif",
		Attachment: true,
	}); err != nil {
		t.Fatalf("serve user file: %v", err)
	}

	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Errorf("Content-Disposition: got %q, want attachment", got)
	}
}

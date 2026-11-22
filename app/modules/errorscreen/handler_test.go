package errorscreen_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"kumacore/app/modules/errorscreen"
)

type mockRenderer struct {
	renderErrorCalls int
	lastStatus       int
	lastMessage      string
}

func (mock *mockRenderer) Render(
	writer http.ResponseWriter,
	request *http.Request,
	pageFile string,
	fragment string,
	data any,
) error {
	return nil
}

func (mock *mockRenderer) RenderError(writer http.ResponseWriter, request *http.Request, status int, message string) {
	mock.renderErrorCalls++
	mock.lastStatus = status
	mock.lastMessage = message
	writer.WriteHeader(status)
}

func TestNotFound_DelegatesToRenderErrorWith404AndLs(t *testing.T) {
	renderer := &mockRenderer{}

	recorder := httptest.NewRecorder()
	errorscreen.NewHandler(renderer).NotFound(recorder, httptest.NewRequest(http.MethodGet, "/missing", nil))

	if renderer.renderErrorCalls != 1 {
		t.Errorf("RenderError calls: got %d, want 1", renderer.renderErrorCalls)
	}
	if renderer.lastStatus != http.StatusNotFound {
		t.Errorf("RenderError status: got %d, want %d", renderer.lastStatus, http.StatusNotFound)
	}
	if renderer.lastMessage != "ls" {
		t.Errorf("RenderError message: got %q, want %q", renderer.lastMessage, "ls")
	}
}

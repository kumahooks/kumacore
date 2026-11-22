package home_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"kumacore/app/modules/home"
)

type mockRenderer struct {
	lastPageFile string
	lastFragment string
	lastData     any
	err          error
}

func (mock *mockRenderer) Render(
	writer http.ResponseWriter,
	request *http.Request,
	pageFile string,
	fragment string,
	data any,
) error {
	mock.lastPageFile = pageFile
	mock.lastFragment = fragment
	mock.lastData = data

	return mock.err
}

func (mock *mockRenderer) RenderError(writer http.ResponseWriter, request *http.Request, status int, message string) {
	writer.WriteHeader(status)
}

func TestIndex_RendersHomeContract(t *testing.T) {
	renderer := &mockRenderer{}
	handler := home.NewHandler(renderer, "test")

	recorder := httptest.NewRecorder()
	handler.Index(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusOK)
	}

	if renderer.lastPageFile != "modules/home/home.html" {
		t.Errorf("page file: got %q, want modules/home/home.html", renderer.lastPageFile)
	}

	if renderer.lastFragment != "page-content" {
		t.Errorf("fragment: got %q, want page-content", renderer.lastFragment)
	}

	dataMap, ok := renderer.lastData.(map[string]any)
	if !ok {
		t.Fatal("render data is not map[string]any")
	}

	if title := dataMap["Title"]; title != "test" {
		t.Errorf("title: got %v, want test", title)
	}

	if appName := dataMap["AppName"]; appName != "test" {
		t.Errorf("app name: got %v, want test", appName)
	}

	if _, hasPage := dataMap["Page"]; hasPage {
		t.Error("unexpected Page key for home")
	}

	if _, hasUser := dataMap["User"]; hasUser {
		t.Error("unexpected User key in render data")
	}
}

func TestIndex_RendererError_Returns500(t *testing.T) {
	renderer := &mockRenderer{err: errors.New("template failure")}
	handler := home.NewHandler(renderer, "test")

	recorder := httptest.NewRecorder()
	handler.Index(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	if body := recorder.Body.String(); body != "internal server error\n" {
		t.Errorf("body: got %q, want %q (no error leak to client)", body, "internal server error\n")
	}
}

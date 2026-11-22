package render_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"kumacore/core/render"
)

var testFS = fstest.MapFS{
	"web/templates/layouts/base.html": {
		Data: []byte(`{{define "base"}}base:{{template "page-content" .}}{{end}}`),
	},
	"web/templates/components/navbar.html": {
		Data: []byte(`{{define "navbar"}}{{end}}`),
	},
	"modules/page/page.html": {
		Data: []byte(`{{define "page-content"}}content:{{.Title}}{{end}}`),
	},
	"modules/other/other.html": {
		Data: []byte(`{{define "page-content"}}other:{{.Title}}{{end}}`),
	},
	"modules/error/error.html": {
		Data: []byte(`{{define "page-content"}}{{template "error-body" .}}{{end}}`),
	},
	"modules/error/components/error-body.html": {
		Data: []byte(`{{define "error-body"}}error:{{.Status}}:{{.Message}}{{end}}`),
	},
}

const errorTemplatePath = "modules/error/error.html"

func TestNewManagerMissingLayoutsReturnsError(t *testing.T) {
	_, err := render.NewManager(false, fstest.MapFS{}, errorTemplatePath)
	if err == nil {
		t.Fatal("expected error for missing layout templates")
	}
}

func TestNewManagerEmptyErrorTemplatePathReturnsError(t *testing.T) {
	_, err := render.NewManager(false, testFS, "")
	if err == nil {
		t.Fatal("expected error for empty errorTemplatePath")
	}
}

func TestNewManagerProductionRejectsMissingErrorTemplate(t *testing.T) {
	_, err := render.NewManager(false, testFS, "modules/missing/missing.html")
	if err == nil {
		t.Fatal("expected error for missing error template in production")
	}
}

func TestNewManagerDevAcceptsMissingErrorTemplate(t *testing.T) {
	_, err := render.NewManager(true, testFS, "modules/missing/missing.html")
	if err != nil {
		t.Fatalf("dev mode should defer error template validation, got: %v", err)
	}
}

func TestNewManagerProductionRejectsInvalidErrorModuleComponent(t *testing.T) {
	fileSystem := fstest.MapFS{
		"web/templates/layouts/base.html": {
			Data: []byte(`{{define "base"}}base:{{template "page-content" .}}{{end}}`),
		},
		"web/templates/components/navbar.html": {
			Data: []byte(`{{define "navbar"}}{{end}}`),
		},
		"modules/error/error.html": {
			Data: []byte(`{{define "page-content"}}{{template "error-body" .}}{{end}}`),
		},
		"modules/error/components/first.html": {
			Data: []byte(`{{define "error-body"}}first{{end}}`),
		},
		"modules/error/components/second.html": {
			Data: []byte(`{{define "error-body"}}second{{end}}`),
		},
	}

	_, err := render.NewManager(false, fileSystem, "modules/error/error.html")
	if err == nil || !strings.Contains(err.Error(), `duplicate define "error-body"`) {
		t.Fatalf("NewManager: got %v, want duplicate define error", err)
	}
}

func TestRenderDirectRequestRendersFullPage(t *testing.T) {
	manager, err := render.NewManager(false, testFS, errorTemplatePath)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := manager.Render(
		recorder,
		request,
		"modules/page/page.html",
		"page-content",
		map[string]any{"Title": "hello"},
	); err != nil {
		t.Fatalf("Render: %v", err)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "base:") {
		t.Errorf("expected full page render, got: %q", body)
	}

	if !strings.Contains(body, "content:hello") {
		t.Errorf("expected page content in body, got: %q", body)
	}

	if vary := recorder.Header().Values("Vary"); !slices.Contains(vary, "HX-Request-Type") {
		t.Errorf("expected Vary to contain HX-Request-Type, got: %v", vary)
	}
}

func TestRenderHTMXFragmentRendersFragmentOnly(t *testing.T) {
	manager, err := render.NewManager(false, testFS, errorTemplatePath)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("HX-Request-Type", "partial")

	if err := manager.Render(
		recorder,
		request,
		"modules/page/page.html",
		"page-content",
		map[string]any{"Title": "hello"},
	); err != nil {
		t.Fatalf("Render: %v", err)
	}

	body := recorder.Body.String()
	if strings.Contains(body, "base:") {
		t.Errorf("expected fragment only, got full page: %q", body)
	}

	if !strings.Contains(body, "content:hello") {
		t.Errorf("expected fragment content in body, got: %q", body)
	}

	if vary := recorder.Header().Values("Vary"); !slices.Contains(vary, "HX-Request-Type") {
		t.Errorf("expected Vary to contain HX-Request-Type, got: %v", vary)
	}
}

func TestRenderHistoryRestoreRendersFullPage(t *testing.T) {
	manager, err := render.NewManager(false, testFS, errorTemplatePath)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	// HX-Request-Type is intentionally absent: real direct-load and history-restore requests carry "full" or no htmx header
	// at all. Only partial requests are fragments.

	if err := manager.Render(
		recorder,
		request,
		"modules/page/page.html",
		"page-content",
		map[string]any{"Title": "restore"},
	); err != nil {
		t.Fatalf("Render: %v", err)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "base:") {
		t.Errorf("expected full page render on history restore, got: %q", body)
	}

	if !strings.Contains(body, "content:restore") {
		t.Errorf("expected page content in body, got: %q", body)
	}
}

func TestRender_HTMXBoosted_RendersFragmentOnly(t *testing.T) {
	manager, err := render.NewManager(false, testFS, errorTemplatePath)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("HX-Request-Type", "partial")
	request.Header.Set("HX-Boosted", "true")

	if err := manager.Render(
		recorder,
		request,
		"modules/page/page.html",
		"page-content",
		map[string]any{"Title": "hello"},
	); err != nil {
		t.Fatalf("Render: %v", err)
	}

	body := recorder.Body.String()
	if strings.Contains(body, "base:") {
		t.Errorf("expected fragment only for boosted request, got full page: %q", body)
	}

	if !strings.Contains(body, "content:hello") {
		t.Errorf("expected fragment content in body, got: %q", body)
	}
}

func TestRenderMissingPageFileReturnsError(t *testing.T) {
	manager, err := render.NewManager(false, testFS, errorTemplatePath)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	err = manager.Render(recorder, request, "modules/missing/missing.html", "page-content", nil)
	if err == nil {
		t.Fatal("expected error for missing page file")
	}
}

func TestRenderCloneIsolationPagesDoNotCollide(t *testing.T) {
	manager, err := render.NewManager(false, testFS, errorTemplatePath)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorderA := httptest.NewRecorder()
	if err := manager.Render(
		recorderA,
		httptest.NewRequest(http.MethodGet, "/", nil),
		"modules/page/page.html",
		"page-content",
		map[string]any{"Title": "alpha"},
	); err != nil {
		t.Fatalf("Render page: %v", err)
	}

	recorderB := httptest.NewRecorder()
	if err := manager.Render(
		recorderB,
		httptest.NewRequest(http.MethodGet, "/", nil),
		"modules/other/other.html",
		"page-content",
		map[string]any{"Title": "beta"},
	); err != nil {
		t.Fatalf("Render other: %v", err)
	}

	if bodyA := recorderA.Body.String(); !strings.Contains(bodyA, "content:alpha") ||
		strings.Contains(bodyA, "other:") {
		t.Errorf("page render contaminated: %q", bodyA)
	}

	if bodyB := recorderB.Body.String(); !strings.Contains(bodyB, "other:beta") || strings.Contains(bodyB, "content:") {
		t.Errorf("other render contaminated: %q", bodyB)
	}
}

func TestRenderDevModeRendersCorrectly(t *testing.T) {
	manager, err := render.NewManager(true, testFS, errorTemplatePath)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := manager.Render(
		recorder,
		request,
		"modules/page/page.html",
		"page-content",
		map[string]any{"Title": "devmode"},
	); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if !strings.Contains(recorder.Body.String(), "content:devmode") {
		t.Errorf("dev mode render failed: %q", recorder.Body.String())
	}
}

func TestRenderErrorDirectRequestRendersFullPageWithStatus(t *testing.T) {
	manager, err := render.NewManager(false, testFS, errorTemplatePath)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	manager.RenderError(recorder, request, http.StatusInternalServerError, "boom")

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "base:") {
		t.Errorf("expected full page render, got: %q", body)
	}

	if !strings.Contains(body, "error:500:boom") {
		t.Errorf("expected error content in body, got: %q", body)
	}

	if vary := recorder.Header().Values("Vary"); !slices.Contains(vary, "HX-Request-Type") {
		t.Errorf("expected Vary to contain HX-Request-Type, got: %v", vary)
	}
}

func TestRenderErrorHTMXRequestSetsHXRedirectWithStatus(t *testing.T) {
	manager, err := render.NewManager(false, testFS, errorTemplatePath)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/errorscreen", nil)
	request.Header.Set("HX-Request-Type", "partial")

	manager.RenderError(recorder, request, http.StatusBadRequest, "bad input")

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	if got := recorder.Header().Get("HX-Redirect"); got != "/errorscreen" {
		t.Errorf("HX-Redirect: got %q, want %q", got, "/errorscreen")
	}

	if body := recorder.Body.String(); body != "" {
		t.Errorf("body: got %q, want empty (HX-Redirect path carries no body)", body)
	}

	if vary := recorder.Header().Values("Vary"); !slices.Contains(vary, "HX-Request-Type") {
		t.Errorf("expected Vary to contain HX-Request-Type, got: %v", vary)
	}
}

func TestRenderErrorHTMXRequestPreservesQueryStringInHXRedirect(t *testing.T) {
	manager, err := render.NewManager(false, testFS, errorTemplatePath)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/errorscreen?lain=god", nil)
	request.Header.Set("HX-Request-Type", "partial")

	manager.RenderError(recorder, request, http.StatusNotFound, "ls")

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusNotFound)
	}

	if got := recorder.Header().Get("HX-Redirect"); got != "/errorscreen?lain=god" {
		t.Errorf("HX-Redirect: got %q, want %q", got, "/errorscreen?lain=god")
	}

	if body := recorder.Body.String(); body != "" {
		t.Errorf("body: got %q, want empty (HX-Redirect path carries no body)", body)
	}
}

func TestRenderErrorHTMXHistoryRestoreRequestRendersFullPage(t *testing.T) {
	manager, err := render.NewManager(false, testFS, errorTemplatePath)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/errorscreen", nil)
	request.Header.Set("HX-Request-Type", "full")
	request.Header.Set("HX-History-Restore-Request", "true")

	manager.RenderError(recorder, request, http.StatusNotFound, "ls")

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusNotFound)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "base:") {
		t.Errorf("expected full page render for history restore, got: %q", body)
	}

	if !strings.Contains(body, "error:404:ls") {
		t.Errorf("expected error content in body, got: %q", body)
	}
}

func TestRenderErrorMissingTemplateFallsBackToHTTPError(t *testing.T) {
	manager, err := render.NewManager(true, testFS, "modules/missing/missing.html")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	manager.RenderError(recorder, request, http.StatusInternalServerError, "fallback")

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "fallback") {
		t.Errorf("expected http.Error fallback body, got: %q", body)
	}
}

func TestRenderModuleComponentIsParsedWithPage(t *testing.T) {
	fileSystem := fstest.MapFS{
		"web/templates/layouts/base.html": {
			Data: []byte(`{{define "base"}}base:{{template "page-content" .}}{{end}}`),
		},
		"web/templates/components/navbar.html": {
			Data: []byte(`{{define "navbar"}}{{end}}`),
		},
		"modules/widget/widget.html": {
			Data: []byte(`{{define "page-content"}}{{template "widget-body" .}}{{end}}`),
		},
		"modules/widget/components/widget-body.html": {
			Data: []byte(`{{define "widget-body"}}widget:{{.Title}}{{end}}`),
		},
		"modules/error/error.html": {
			Data: []byte(`{{define "page-content"}}error:{{.Status}}:{{.Message}}{{end}}`),
		},
	}

	manager, err := render.NewManager(false, fileSystem, "modules/error/error.html")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	if err := manager.Render(
		recorder,
		httptest.NewRequest(http.MethodGet, "/", nil),
		"modules/widget/widget.html",
		"page-content",
		map[string]any{"Title": "hello"},
	); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if body := recorder.Body.String(); !strings.Contains(body, "widget:hello") {
		t.Errorf("expected module component output, got: %q", body)
	}
}

func TestRenderDuplicateModuleComponentDefineReturnsError(t *testing.T) {
	fileSystem := fstest.MapFS{
		"web/templates/layouts/base.html": {
			Data: []byte(`{{define "base"}}base:{{template "page-content" .}}{{end}}`),
		},
		"web/templates/components/navbar.html": {
			Data: []byte(`{{define "navbar"}}{{end}}`),
		},
		"modules/duplicate/duplicate.html": {
			Data: []byte(`{{define "page-content"}}duplicate{{end}}`),
		},
		"modules/duplicate/components/first.html": {
			Data: []byte(`{{define "shared"}}first{{end}}`),
		},
		"modules/duplicate/components/second.html": {
			Data: []byte(`{{define "shared"}}second{{end}}`),
		},
		"modules/error/error.html": {
			Data: []byte(`{{define "page-content"}}error:{{.Status}}:{{.Message}}{{end}}`),
		},
	}

	manager, err := render.NewManager(false, fileSystem, "modules/error/error.html")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	err = manager.Render(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/", nil),
		"modules/duplicate/duplicate.html",
		"page-content",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), `duplicate define "shared"`) {
		t.Fatalf("Render: got %v, want duplicate define error", err)
	}
}

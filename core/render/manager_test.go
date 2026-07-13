package render_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"kumacore/core/render"
)

var testFS = fstest.MapFS{
	"app/web/templates/layouts/base.html": {
		Data: []byte(`{{define "base"}}base:{{template "page-content" .}}{{end}}`),
	},
	"app/web/templates/components/navbar.html": {
		Data: []byte(`{{define "navbar"}}{{end}}`),
	},
	"app/modules/page/page.html": {
		Data: []byte(`{{define "page-content"}}content:{{.Title}}{{end}}`),
	},
	"app/modules/other/other.html": {
		Data: []byte(`{{define "page-content"}}other:{{.Title}}{{end}}`),
	},
}

func TestNewManagerMissingLayoutsReturnsError(t *testing.T) {
	_, err := render.NewManager(false, fstest.MapFS{})
	if err == nil {
		t.Fatal("expected error for missing layout templates")
	}
}

func TestRenderDirectRequestRendersFullPage(t *testing.T) {
	manager, err := render.NewManager(false, testFS)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := manager.Render(
		recorder,
		request,
		"app/modules/page/page.html",
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
}

func TestRenderHTMXFragmentRendersFragmentOnly(t *testing.T) {
	manager, err := render.NewManager(false, testFS)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("HX-Request", "true")

	if err := manager.Render(
		recorder,
		request,
		"app/modules/page/page.html",
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
}

func TestRenderHistoryRestoreRendersFullPage(t *testing.T) {
	manager, err := render.NewManager(false, testFS)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("HX-Request", "true")
	request.Header.Set("HX-History-Restore-Request", "true")

	if err := manager.Render(
		recorder,
		request,
		"app/modules/page/page.html",
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
	manager, err := render.NewManager(false, testFS)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("HX-Request", "true")
	request.Header.Set("HX-Boosted", "true")

	if err := manager.Render(
		recorder,
		request,
		"app/modules/page/page.html",
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
	manager, err := render.NewManager(false, testFS)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	err = manager.Render(recorder, request, "app/modules/missing/missing.html", "page-content", nil)
	if err == nil {
		t.Fatal("expected error for missing page file")
	}
}

func TestRenderCloneIsolationPagesDoNotCollide(t *testing.T) {
	manager, err := render.NewManager(false, testFS)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorderA := httptest.NewRecorder()
	if err := manager.Render(
		recorderA,
		httptest.NewRequest(http.MethodGet, "/", nil),
		"app/modules/page/page.html",
		"page-content",
		map[string]any{"Title": "alpha"},
	); err != nil {
		t.Fatalf("Render page: %v", err)
	}

	recorderB := httptest.NewRecorder()
	if err := manager.Render(
		recorderB,
		httptest.NewRequest(http.MethodGet, "/", nil),
		"app/modules/other/other.html",
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
	manager, err := render.NewManager(true, testFS)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := manager.Render(
		recorder,
		request,
		"app/modules/page/page.html",
		"page-content",
		map[string]any{"Title": "devmode"},
	); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if !strings.Contains(recorder.Body.String(), "content:devmode") {
		t.Errorf("dev mode render failed: %q", recorder.Body.String())
	}
}

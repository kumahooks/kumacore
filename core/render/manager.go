// Package render manages HTML template parsing and rendering.
//
// In production (dev=false), templates are parsed once at startup and cached for the lifetime of the process. Page
// templates are cloned from the shared base set on first use to prevent naming collisions. In dev mode (dev=true),
// templates are re-parsed from disk on every request so file changes are picked up without restarting.
//
// The caller provides an fs.FS that backs all template reads. It is the caller's responsibility to choose between an
// embedded FS (binary-baked in production) and a disk-backed FS (os.DirFS in dev).
//
// Template layers, parsed in order:
//   - layouts: shared full-page shells (web/templates/layouts/*.html)
//   - components: shared cross-module fragments (web/templates/components/*.html)
//   - module components: per-module fragments (modules/<name>/components/*.html),
//     derived from the directory of the page file. Modules without a components
//     directory are unaffected. Module component files must not define
//     "page-content". That block belongs to the page file.
//   - page file: the single file passed to Render, defining "page-content" and
//     any module-specific blocks.
//
// All layers share one flat template namespace within a parsed set. Define names must be unique within the combined set
// of a single build.
package render

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"sync"
)

const (
	layoutsPattern    = "web/templates/layouts/*.html"
	componentsPattern = "web/templates/components/*.html"
)

// Renderer is the interface handlers use to execute page templates.
type Renderer interface {
	Render(writer http.ResponseWriter, request *http.Request, pageFile string, fragment string, data any) error
}

// Manager holds the base template set, layouts and components, and an fs.FS pointing to the app root.
// prod: generated app filesystem with page cache
// dev: generated app filesystem without page cache
type Manager struct {
	baseTemplate   *template.Template
	fileSystem     fs.FS
	pageCache      map[string]*template.Template
	pageCacheMutex sync.RWMutex
	isDevelopment  bool
}

// NewManager parses the shared base templates, layouts and components, from fileSystem and returns a Manager ready to
// render pages. isDevelopment disables the page cache so templates are re-parsed from disk on every request.
func NewManager(isDevelopment bool, fileSystem fs.FS) (*Manager, error) {
	baseTemplate := template.New("")

	if _, err := baseTemplate.ParseFS(fileSystem, layoutsPattern); err != nil {
		return nil, fmt.Errorf("[render:NewManager] parse layouts: %w", err)
	}

	if _, err := baseTemplate.ParseFS(fileSystem, componentsPattern); err != nil {
		return nil, fmt.Errorf("[render:NewManager] parse components: %w", err)
	}

	return &Manager{
		baseTemplate:  baseTemplate,
		fileSystem:    fileSystem,
		pageCache:     make(map[string]*template.Template),
		isDevelopment: isDevelopment,
	}, nil
}

// Render executes the page template against the request, returning either a full page or a fragment depending on the
// HX-Request-Type header (htmx 4).
//
// fragment is the template name executed for partial HTMX requests (HX-Request-Type "partial"). Full page requests
// always execute "base". Standard page handlers pass "page-content". Fine-grained fragment endpoints pass the specific
// named define they want to render.
func (templateManager *Manager) Render(
	writer http.ResponseWriter,
	request *http.Request,
	pageFile string,
	fragment string,
	data any,
) error {
	pageTemplate, err := templateManager.resolve(pageFile)
	if err != nil {
		return err
	}

	if request.Header.Get("HX-History-Restore-Request") == "true" {
		return pageTemplate.ExecuteTemplate(writer, "base", data)
	}

	if request.Header.Get("HX-Request") == "true" {
		return pageTemplate.ExecuteTemplate(writer, fragment, data)
	}

	return pageTemplate.ExecuteTemplate(writer, "base", data)
}

// resolve returns a cached template in production or builds a fresh one in dev mode.
func (templateManager *Manager) resolve(pageFile string) (*template.Template, error) {
	if templateManager.isDevelopment {
		return templateManager.build(pageFile)
	}

	templateManager.pageCacheMutex.RLock()
	cachedTemplate, ok := templateManager.pageCache[pageFile]
	templateManager.pageCacheMutex.RUnlock()
	if ok {
		return cachedTemplate, nil
	}

	templateManager.pageCacheMutex.Lock()
	defer templateManager.pageCacheMutex.Unlock()

	if cachedTemplate, ok = templateManager.pageCache[pageFile]; ok {
		return cachedTemplate, nil
	}

	builtTemplate, err := templateManager.build(pageFile)
	if err != nil {
		return nil, err
	}

	templateManager.pageCache[pageFile] = builtTemplate

	return builtTemplate, nil
}

// build constructs a complete template set for pageFile.
//
// In dev mode all three layers, layouts, components, and page, are parsed fresh from disk on every call so any HTML
// change is picked up without a restart. In production the pre-parsed base is cloned and only the page file is parsed
// into the clone.
func (templateManager *Manager) build(pageFile string) (*template.Template, error) {
	if templateManager.isDevelopment {
		freshTemplate := template.New("")

		if _, err := freshTemplate.ParseFS(templateManager.fileSystem, layoutsPattern); err != nil {
			return nil, fmt.Errorf("[render:build] parse layouts: %w", err)
		}

		if _, err := freshTemplate.ParseFS(templateManager.fileSystem, componentsPattern); err != nil {
			return nil, fmt.Errorf("[render:build] parse components: %w", err)
		}

		if _, err := freshTemplate.ParseFS(templateManager.fileSystem, pageFile); err != nil {
			return nil, fmt.Errorf("[render:build] parse page template %q: %w", pageFile, err)
		}

		return freshTemplate, nil
	}

	clonedTemplate, err := templateManager.baseTemplate.Clone()
	if err != nil {
		return nil, fmt.Errorf("[render:build] clone base template set: %w", err)
	}

	if _, err = clonedTemplate.ParseFS(templateManager.fileSystem, pageFile); err != nil {
		return nil, fmt.Errorf("[render:build] parse page template %q: %w", pageFile, err)
	}

	return clonedTemplate, nil
}

// Package render manages HTML template parsing and HTMX-aware rendering.
//
// In production, templates are served from the filesystem passed by generated
// app bootstrap and page templates are cached.
// In dev mode, templates are read from disk on every request so file changes
// are picked up without restarting.
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

// Manager holds the base template set, layouts and components, and an fs.FS
// pointing to the generated app root.
// prod: generated app filesystem with page cache
// dev: generated app filesystem without page cache
type Manager struct {
	baseTemplate   *template.Template
	fileSystem     fs.FS
	pageCache      map[string]*template.Template
	pageCacheMutex sync.RWMutex
	isDevelopment  bool
}

// NewManager parses the shared base templates, layouts and components, from fileSystem
// and returns a Manager ready to render pages. isDevelopment disables the page cache
// so templates are re-parsed from disk on every request.
func NewManager(isDevelopment bool, fileSystem fs.FS) (*Manager, error) {
	baseTemplate := template.New("")

	if _, err := baseTemplate.ParseFS(fileSystem, layoutsPattern); err != nil {
		return nil, err
	}

	if _, err := baseTemplate.ParseFS(fileSystem, componentsPattern); err != nil {
		return nil, err
	}

	return &Manager{
		baseTemplate:  baseTemplate,
		fileSystem:    fileSystem,
		pageCache:     make(map[string]*template.Template),
		isDevelopment: isDevelopment,
	}, nil
}

// Render executes the page template against the request, returning either a
// full page or a fragment depending on HTMX headers.
//
// fragment is the template name executed for HTMX requests. Full page requests
// always execute "base". Standard page handlers pass "page-content";
// fine-grained fragment endpoints pass the specific named define they want to render.
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
	cached, ok := templateManager.pageCache[pageFile]
	templateManager.pageCacheMutex.RUnlock()
	if ok {
		return cached, nil
	}

	templateManager.pageCacheMutex.Lock()
	defer templateManager.pageCacheMutex.Unlock()

	if cached, ok = templateManager.pageCache[pageFile]; ok {
		return cached, nil
	}

	built, err := templateManager.build(pageFile)
	if err != nil {
		return nil, err
	}

	templateManager.pageCache[pageFile] = built
	return built, nil
}

// build constructs a complete template set for pageFile.
//
// In dev mode all three layers, layouts, components, and page, are parsed fresh
// from disk on every call so any HTML change is picked up without a restart.
// In production the pre-parsed base is cloned and only the page file is parsed.
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

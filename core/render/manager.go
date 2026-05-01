// Package render manages HTML template parsing and HTMX-aware rendering.
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

// Manager holds the base template set and app filesystem.
type Manager struct {
	baseTemplate   *template.Template
	fileSystem     fs.FS
	pageCache      map[string]*template.Template
	pageCacheMutex sync.RWMutex
	isDevelopment  bool
}

// NewManager parses shared layouts and components and returns a Manager ready to render pages.
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

// Render executes the page template against the request.
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

func (templateManager *Manager) build(pageFile string) (*template.Template, error) {
	if templateManager.isDevelopment {
		freshTemplate := template.New("")

		if _, err := freshTemplate.ParseFS(templateManager.fileSystem, layoutsPattern); err != nil {
			return nil, fmt.Errorf("[render] parse layouts: %w", err)
		}

		if _, err := freshTemplate.ParseFS(templateManager.fileSystem, componentsPattern); err != nil {
			return nil, fmt.Errorf("[render] parse components: %w", err)
		}

		if _, err := freshTemplate.ParseFS(templateManager.fileSystem, pageFile); err != nil {
			return nil, fmt.Errorf("[render] parse page template %q: %w", pageFile, err)
		}

		return freshTemplate, nil
	}

	clonedTemplate, err := templateManager.baseTemplate.Clone()
	if err != nil {
		return nil, fmt.Errorf("[render] clone base template set: %w", err)
	}

	if _, err = clonedTemplate.ParseFS(templateManager.fileSystem, pageFile); err != nil {
		return nil, fmt.Errorf("[render] parse page template %q: %w", pageFile, err)
	}

	return clonedTemplate, nil
}

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
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"sync"
)

const (
	layoutsPattern    = "web/templates/layouts/*.html"
	componentsPattern = "web/templates/components/*.html"
)

// Renderer is the interface handlers use to execute page templates. RenderError renders the styled not-found error
// screen and writes the given status, falling back to http.Error if the error template itself fails to render.
type Renderer interface {
	Render(writer http.ResponseWriter, request *http.Request, pageFile string, fragment string, data any) error
	RenderError(writer http.ResponseWriter, request *http.Request, status int, message string)
}

// Manager holds the base template set, layouts and components, and an fs.FS pointing to the app root.
// prod: generated app filesystem with page cache
// dev: generated app filesystem without page cache
type Manager struct {
	baseTemplate      *template.Template
	fileSystem        fs.FS
	errorTemplatePath string
	pageCache         map[string]*template.Template
	pageCacheMutex    sync.RWMutex
	isDevelopment     bool
}

// NewManager parses the shared base templates, layouts and components, from fileSystem and returns a Manager ready to
// render pages. isDevelopment disables the page cache so templates are re-parsed from disk on every request. errorTemplatePath
// is the page template path used by RenderError, relative to fileSystem.
//
// errorTemplatePath must be non-empty. In production it is also parsed against fileSystem at startup, so a misconfigured
// embed fails fast at boot instead of degrading to http.Error on the first error request. In dev mode the file is re-read
// on every RenderError call, so a missing file is reported through the RenderError fallback path.
func NewManager(isDevelopment bool, fileSystem fs.FS, errorTemplatePath string) (*Manager, error) {
	if errorTemplatePath == "" {
		return nil, fmt.Errorf("[render:NewManager] errorTemplatePath is required")
	}

	baseTemplate := template.New("")

	if _, err := baseTemplate.ParseFS(fileSystem, layoutsPattern); err != nil {
		return nil, fmt.Errorf("[render:NewManager] parse layouts: %w", err)
	}

	if _, err := baseTemplate.ParseFS(fileSystem, componentsPattern); err != nil {
		return nil, fmt.Errorf("[render:NewManager] parse components: %w", err)
	}

	templateManager := &Manager{
		baseTemplate:      baseTemplate,
		fileSystem:        fileSystem,
		errorTemplatePath: errorTemplatePath,
		pageCache:         make(map[string]*template.Template),
		isDevelopment:     isDevelopment,
	}

	if !isDevelopment {
		if _, err := templateManager.build(errorTemplatePath); err != nil {
			return nil, fmt.Errorf("[render:NewManager] parse error template %q: %w", errorTemplatePath, err)
		}
	}

	return templateManager, nil
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

	writer.Header().Add("Vary", "HX-Request-Type")

	return templateManager.executeTemplate(writer, pageTemplate, request, fragment, data)
}

// RenderError writes the given HTTP status and renders the configured error template as a full base page. The error
// screen is always a full document.
//
// For HTMX requests (boosted navigation, partial polls), a full document cannot be swapped into #content, so RenderError
// sets HX-Redirect to the request URL. htmx then performs a full-page navigation that re-enters the handler as a direct
// load, which renders base. The URL the user sees is the one that failed, so refresh and sharing keep working.
//
// Rendering happens into an in-memory buffer first. If the template fails partway through, no status header or partial
// body has been written yet, so RenderError falls back to http.Error with a clean response. Only on success does it
// write the status header and the buffered body.
func (templateManager *Manager) RenderError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	message string,
) {
	if request.Header.Get("HX-Request-Type") == "partial" {
		writer.Header().Add("Vary", "HX-Request-Type")
		writer.Header().Set("HX-Redirect", request.URL.RequestURI())
		writer.WriteHeader(status)
		return
	}

	data := map[string]any{
		"Title":   fmt.Sprintf("%d - %s", status, http.StatusText(status)),
		"Status":  status,
		"Message": message,
	}

	pageTemplate, err := templateManager.resolve(templateManager.errorTemplatePath)
	if err != nil {
		slog.ErrorContext(
			request.Context(), "resolve error template",
			slog.String("caller", "render:RenderError"),
			slog.Any("err", err),
		)

		http.Error(writer, message, status)

		return
	}

	var buffer bytes.Buffer
	if err := templateManager.executeTemplate(&buffer, pageTemplate, request, "page-content", data); err != nil {
		slog.ErrorContext(
			request.Context(), "render error template",
			slog.String("caller", "render:RenderError"),
			slog.Any("err", err),
		)

		http.Error(writer, message, status)

		return
	}

	writer.Header().Add("Vary", "HX-Request-Type")
	writer.WriteHeader(status)
	_, _ = writer.Write(buffer.Bytes())
}

// executeTemplate picks the template name based on the same three-way dispatch used by Render: direct loads and history
// restores (HTMX-Request-Type "full") execute "base". Partial HTMX requests (HX-Request-Type "partial") execute the
// named fragment. The writer is any io.Writer so the same path can render into an HTTP response or an in-memory buffer.
func (templateManager *Manager) executeTemplate(
	writer io.Writer,
	pageTemplate *template.Template,
	request *http.Request,
	fragment string,
	data any,
) error {
	if request.Header.Get("HX-Request-Type") == "partial" {
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
// In dev mode all four layers, layouts, components, module-local components, and page, are parsed fresh from disk on
// every call so any HTML change is picked up without a restart. In production the pre-parsed base is cloned and only
// the module-local components and page file are parsed into the clone.
//
// Module-local components live under modules/<name>/components/*.html, derived from the directory of pageFile. They
// hold named fragment defines that the page file references via {{template "name" .}}. This keeps large modules split
// across small files without polluting the shared global component namespace. Modules without a components directory
// are unaffected: fs.Glob returns an empty slice and the step is skipped.
func (templateManager *Manager) build(pageFile string) (*template.Template, error) {
	moduleComponents, err := moduleComponentFiles(templateManager.fileSystem, pageFile)
	if err != nil {
		return nil, err
	}

	if templateManager.isDevelopment {
		freshTemplate := template.New("")

		if _, err = freshTemplate.ParseFS(templateManager.fileSystem, layoutsPattern); err != nil {
			return nil, fmt.Errorf("[render:build] parse layouts: %w", err)
		}

		if _, err = freshTemplate.ParseFS(templateManager.fileSystem, componentsPattern); err != nil {
			return nil, fmt.Errorf("[render:build] parse components: %w", err)
		}

		// len > 0 guard is required: ParseFS errors on an empty file list ("no files named in call to ParseFiles"). fs.Glob
		// returns an empty slice for modules without a components directory.
		if len(moduleComponents) > 0 {
			if err = validateModuleComponentNames(templateManager.fileSystem, moduleComponents); err != nil {
				return nil, err
			}

			if _, err = freshTemplate.ParseFS(templateManager.fileSystem, moduleComponents...); err != nil {
				return nil, fmt.Errorf("[render:build] parse module components: %w", err)
			}
		}

		if _, err = freshTemplate.ParseFS(templateManager.fileSystem, pageFile); err != nil {
			return nil, fmt.Errorf("[render:build] parse page template %q: %w", pageFile, err)
		}

		return freshTemplate, nil
	}

	clonedTemplate, err := templateManager.baseTemplate.Clone()
	if err != nil {
		return nil, fmt.Errorf("[render:build] clone base template set: %w", err)
	}

	if len(moduleComponents) > 0 {
		if err := validateModuleComponentNames(templateManager.fileSystem, moduleComponents); err != nil {
			return nil, err
		}

		if _, err := clonedTemplate.ParseFS(templateManager.fileSystem, moduleComponents...); err != nil {
			return nil, fmt.Errorf("[render:build] parse module components: %w", err)
		}
	}

	if _, err := clonedTemplate.ParseFS(templateManager.fileSystem, pageFile); err != nil {
		return nil, fmt.Errorf("[render:build] parse page template %q: %w", pageFile, err)
	}

	return clonedTemplate, nil
}

// moduleComponentFiles returns the list of module-local component files for the given pageFile, or an empty slice if
// the module has no components directory. pageFile is a path relative to the Manager fileSystem root, e.g.
// "modules/lain/lain.html".
func moduleComponentFiles(fileSystem fs.FS, pageFile string) ([]string, error) {
	moduleDir := path.Dir(pageFile)
	pattern := path.Join(moduleDir, "components", "*.html")

	matches, err := fs.Glob(fileSystem, pattern)
	if err != nil {
		return nil, fmt.Errorf("[render:moduleComponentFiles] glob %q: %w", pattern, err)
	}

	return matches, nil
}

// validateModuleComponentNames parses each module component file individually and returns an error if the same define
// name appears in more than one file. ParseFS silently overwrites duplicate defines (last file wins), so without this
// check a collision would go unnoticed and produce wrong output.
func validateModuleComponentNames(fileSystem fs.FS, files []string) error {
	seen := make(map[string]string)
	for _, file := range files {
		probe, err := template.New("").ParseFS(fileSystem, file)
		if err != nil {
			return fmt.Errorf("[render:validateModuleComponentNames] parse %q: %w", file, err)
		}

		for _, parsed := range probe.Templates() {
			name := parsed.Name()
			if name == "" {
				continue
			}

			if previous, exists := seen[name]; exists {
				return fmt.Errorf(
					"[render:validateModuleComponentNames] duplicate define %q in %q (already in %q)",
					name,
					file,
					previous,
				)
			}

			seen[name] = file
		}
	}

	return nil
}

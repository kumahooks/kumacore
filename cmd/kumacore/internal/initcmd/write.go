package initcmd

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	serverMainPath = "cmd/server/main.go"
	goModPath      = "go.mod"
	goSumPath      = "go.sum"
	gitIgnorePath  = ".gitignore"
	readmePath     = "README.md"
	appGitKeepPath = "app/migrations/sqlite/app/.gitkeep"
)

type fileChange struct {
	path   string
	reason string
	data   []byte
}

func initializeRepository(stdout io.Writer, options Options) error {
	plannedChanges, err := plannedChanges(options)
	if err != nil {
		return err
	}

	targetRoot := filepath.Join(".", options.ProjectName)
	if err := ensureProjectDestination(targetRoot); err != nil {
		return err
	}

	parentDirectoryPath := filepath.Dir(targetRoot)
	if err := os.MkdirAll(parentDirectoryPath, 0o755); err != nil {
		return fmt.Errorf("[initcmd:initializeRepository] create parent directory: %w", err)
	}

	stagingDirectoryPath, err := os.MkdirTemp(parentDirectoryPath, ".kumacore-init-*")
	if err != nil {
		return fmt.Errorf("[initcmd:initializeRepository] create staging directory: %w", err)
	}

	defer os.RemoveAll(stagingDirectoryPath)

	projectStagingPath := filepath.Join(stagingDirectoryPath, options.ProjectName)
	for _, change := range plannedChanges {
		targetPath := filepath.Join(projectStagingPath, change.path)
		if err := writeFileAtomic(targetPath, change.data); err != nil {
			return err
		}
	}

	if err := os.Rename(projectStagingPath, targetRoot); err != nil {
		return fmt.Errorf("[initcmd:initializeRepository] move generated project into place: %w", err)
	}

	if err := printChangeSummary(stdout, targetRoot, plannedChanges); err != nil {
		return err
	}

	return nil
}

func plannedChanges(options Options) ([]fileChange, error) {
	serverMainBytes, err := os.ReadFile(serverMainPath)
	if err != nil {
		return nil, fmt.Errorf("[initcmd:plannedChanges] read %s: %w", serverMainPath, err)
	}

	repoMetadata := metadataFromOptions(options)
	rewrittenMain, err := rewriteServerMain(serverMainBytes, repoMetadata, options.ProjectName)
	if err != nil {
		return nil, err
	}

	renderedReadme := []byte(renderReadme(options))
	renderedGitIgnore := []byte(renderGitIgnore())
	rewrittenGoMod, err := renderGoMod(options.ProjectName)
	if err != nil {
		return nil, err
	}
	goSumBytes, err := os.ReadFile(goSumPath)
	if err != nil {
		return nil, fmt.Errorf("[initcmd:plannedChanges] read %s: %w", goSumPath, err)
	}

	changes := make([]fileChange, 0, 64)
	changes = append(changes,
		fileChange{path: goModPath, reason: "write project file", data: rewrittenGoMod},
		fileChange{path: goSumPath, reason: "write project file", data: goSumBytes},
		fileChange{path: gitIgnorePath, reason: "write project file", data: renderedGitIgnore},
		fileChange{path: readmePath, reason: "write project file", data: renderedReadme},
		fileChange{path: serverMainPath, reason: "write generated wiring", data: rewrittenMain},
		fileChange{path: appGitKeepPath, reason: "write project file", data: []byte{}},
	)

	projectFiles, err := plannedProjectFiles(options)
	if err != nil {
		return nil, err
	}
	changes = append(changes, projectFiles...)

	return changes, nil
}

func ensureRepoRoot() error {
	requiredPaths := []string{
		serverMainPath,
		goModPath,
		"app/modules",
		"core/config",
	}

	for _, requiredPath := range requiredPaths {
		if _, err := os.Stat(requiredPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("[initcmd:ensureRepoRoot] required path %q is missing", requiredPath)
			}

			return fmt.Errorf("[initcmd:ensureRepoRoot] stat %s: %w", requiredPath, err)
		}
	}

	return nil
}

func ensureProjectDestination(targetRoot string) error {
	targetInfo, err := os.Stat(targetRoot)
	if err == nil {
		if !targetInfo.IsDir() {
			return fmt.Errorf(
				"[initcmd:ensureProjectDestination] target path %q already exists and is not a directory",
				targetRoot,
			)
		}

		return fmt.Errorf("[initcmd:ensureProjectDestination] target directory %q already exists", targetRoot)
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("[initcmd:ensureProjectDestination] stat %s: %w", targetRoot, err)
	}

	return nil
}

func writeFileAtomic(path string, content []byte) error {
	directoryPath := filepath.Dir(path)
	if err := os.MkdirAll(directoryPath, 0o755); err != nil {
		return fmt.Errorf("[initcmd:writeFileAtomic] create %s: %w", directoryPath, err)
	}

	temporaryFile, err := os.CreateTemp(directoryPath, ".tmp-*")
	if err != nil {
		return fmt.Errorf("[initcmd:writeFileAtomic] create temp file: %w", err)
	}

	temporaryPath := temporaryFile.Name()
	if _, err := temporaryFile.Write(content); err != nil {
		temporaryFile.Close()
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("[initcmd:writeFileAtomic] write temp file: %w", err)
	}

	if err := temporaryFile.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("[initcmd:writeFileAtomic] close temp file: %w", err)
	}

	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("[initcmd:writeFileAtomic] rename temp file: %w", err)
	}

	return nil
}

func printChangeSummary(stdout io.Writer, targetRoot string, changes []fileChange) error {
	if _, err := fmt.Fprintf(stdout, "generated standalone app: %s\n", targetRoot); err != nil {
		return fmt.Errorf("[initcmd:printChangeSummary] write summary: %w", err)
	}

	for _, change := range changes {
		if _, err := fmt.Fprintf(
			stdout,
			"- %s: %s\n",
			change.reason,
			filepath.Join(targetRoot, change.path),
		); err != nil {
			return fmt.Errorf("[initcmd:printChangeSummary] write file change: %w", err)
		}
	}

	return nil
}

func renderGoMod(projectName string) ([]byte, error) {
	goModBytes, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("[initcmd:renderGoMod] read %s: %w", goModPath, err)
	}

	goModSource := string(goModBytes)
	if !strings.HasPrefix(goModSource, "module kumacore\n") {
		return nil, fmt.Errorf("[initcmd:renderGoMod] unexpected module declaration in %s", goModPath)
	}

	return []byte(strings.Replace(goModSource, "module kumacore\n", "module "+projectName+"\n", 1)), nil
}

func plannedProjectFiles(options Options) ([]fileChange, error) {
	selectedPaths := []string{
		"core",
		"app/jobs/logging",
		"app/migrations/sqlite/worker",
		"app/web",
	}

	for _, moduleID := range normalizeModuleIDs(options.Modules) {
		switch moduleID {
		case "home":
			selectedPaths = append(selectedPaths, "app/modules/home")
		case "auth":
			selectedPaths = append(selectedPaths,
				"app/modules/auth",
				"app/middleware/auth",
				"app/repositories/auth",
				"app/services/auth",
				"app/repositories/repositories.go",
				"app/migrations/sqlite/app/0001_create_auth_tables.sql",
			)
		case "health":
			selectedPaths = append(selectedPaths, "app/modules/health")
		}
	}

	uniquePaths := make(map[string]struct{}, len(selectedPaths))
	for _, selectedPath := range selectedPaths {
		uniquePaths[selectedPath] = struct{}{}
	}

	orderedPaths := make([]string, 0, len(uniquePaths))
	for selectedPath := range uniquePaths {
		orderedPaths = append(orderedPaths, selectedPath)
	}
	sort.Strings(orderedPaths)

	changes := make([]fileChange, 0, 64)
	for _, selectedPath := range orderedPaths {
		nextChanges, err := collectSourceFiles(selectedPath, options.ProjectName)
		if err != nil {
			return nil, err
		}

		changes = append(changes, nextChanges...)
	}

	return changes, nil
}

func collectSourceFiles(sourcePath string, projectName string) ([]fileChange, error) {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("[initcmd:collectSourceFiles] stat %s: %w", sourcePath, err)
	}

	if !sourceInfo.IsDir() {
		fileData, err := renderProjectFile(sourcePath, projectName)
		if err != nil {
			return nil, err
		}

		return []fileChange{{
			path:   sourcePath,
			reason: "copy scaffold file",
			data:   fileData,
		}}, nil
	}

	changes := make([]fileChange, 0, 32)
	walkError := filepath.WalkDir(sourcePath, func(path string, directoryEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if directoryEntry.IsDir() {
			return nil
		}

		fileData, err := renderProjectFile(path, projectName)
		if err != nil {
			return err
		}

		changes = append(changes, fileChange{
			path:   path,
			reason: "copy scaffold file",
			data:   fileData,
		})

		return nil
	})
	if walkError != nil {
		return nil, fmt.Errorf("[initcmd:collectSourceFiles] walk %s: %w", sourcePath, walkError)
	}

	sort.Slice(changes, func(leftIndex int, rightIndex int) bool {
		return changes[leftIndex].path < changes[rightIndex].path
	})

	return changes, nil
}

func renderProjectFile(sourcePath string, projectName string) ([]byte, error) {
	fileData, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("[initcmd:renderProjectFile] read %s: %w", sourcePath, err)
	}

	if strings.HasSuffix(sourcePath, ".go") {
		rewrittenSource := rewriteModuleImports(string(fileData), projectName)
		if sourcePath == "core/config/config.go" {
			rewrittenSource = rewriteConfigDefaults(rewrittenSource, projectName)
		}
		return []byte(rewrittenSource), nil
	}

	return bytes.Clone(fileData), nil
}

func rewriteServerMain(source []byte, metadata RepoMetadata, projectName string) ([]byte, error) {
	rewriteSections := []struct {
		beginMarker string
		endMarker   string
		content     string
	}{
		{
			beginMarker: "// kumacore:begin modules-imports",
			endMarker:   "// kumacore:end modules-imports",
			content:     renderModuleImports(metadata),
		},
		{
			beginMarker: "// kumacore:begin jobs-imports",
			endMarker:   "// kumacore:end jobs-imports",
			content:     renderJobImports(metadata),
		},
		{
			beginMarker: "// kumacore:begin modules-setup",
			endMarker:   "// kumacore:end modules-setup",
			content:     renderModuleSetup(metadata),
		},
		{
			beginMarker: "// kumacore:begin modules-middleware",
			endMarker:   "// kumacore:end modules-middleware",
			content:     renderModuleMiddleware(metadata),
		},
		{
			beginMarker: "// kumacore:begin modules-routes",
			endMarker:   "// kumacore:end modules-routes",
			content:     renderModuleRoutes(metadata),
		},
		{
			beginMarker: "// kumacore:begin jobs-register",
			endMarker:   "// kumacore:end jobs-register",
			content:     renderJobRegistrations(),
		},
	}

	rewrittenSource := string(source)
	for _, rewriteSection := range rewriteSections {
		nextSource, err := replaceMarkedRegion(
			rewrittenSource,
			rewriteSection.beginMarker,
			rewriteSection.endMarker,
			rewriteSection.content,
		)
		if err != nil {
			return nil, err
		}

		rewrittenSource = nextSource
	}

	rewrittenSource = stripScaffoldMarkers(rewrittenSource)

	return []byte(rewriteModuleImports(rewrittenSource, projectName)), nil
}

func rewriteModuleImports(source string, projectName string) string {
	return strings.ReplaceAll(source, "kumacore/", projectName+"/")
}

func rewriteConfigDefaults(source string, projectName string) string {
	rewrittenSource := strings.ReplaceAll(
		source,
		`default:"./data/db/kumacore.db"`,
		fmt.Sprintf(`default:"./data/db/%s.db"`, projectName),
	)
	rewrittenSource = strings.ReplaceAll(
		rewrittenSource,
		`default:"./data/db/kumacore_worker.db"`,
		fmt.Sprintf(`default:"./data/db/%s_worker.db"`, projectName),
	)
	rewrittenSource = strings.ReplaceAll(
		rewrittenSource,
		`default:"kumacore"`,
		fmt.Sprintf(`default:"%s"`, projectName),
	)

	return rewrittenSource
}

func replaceMarkedRegion(source string, beginMarker string, endMarker string, content string) (string, error) {
	beginIndex := strings.Index(source, beginMarker)
	if beginIndex == -1 {
		return "", fmt.Errorf("[initcmd:replaceMarkedRegion] missing marker %q", beginMarker)
	}

	if strings.Index(source[beginIndex+len(beginMarker):], beginMarker) != -1 {
		return "", fmt.Errorf("[initcmd:replaceMarkedRegion] duplicate marker %q", beginMarker)
	}

	endIndex := strings.Index(source, endMarker)
	if endIndex == -1 {
		return "", fmt.Errorf("[initcmd:replaceMarkedRegion] missing marker %q", endMarker)
	}

	if strings.Index(source[endIndex+len(endMarker):], endMarker) != -1 {
		return "", fmt.Errorf("[initcmd:replaceMarkedRegion] duplicate marker %q", endMarker)
	}

	if endIndex < beginIndex {
		return "", fmt.Errorf("[initcmd:replaceMarkedRegion] marker order is invalid for %q", beginMarker)
	}

	contentStart := beginIndex + len(beginMarker)
	return source[:contentStart] + "\n" + content + source[endIndex:], nil
}

func stripScaffoldMarkers(source string) string {
	lines := strings.Split(source, "\n")
	filteredLines := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "// kumacore:begin ") || strings.HasPrefix(trimmedLine, "// kumacore:end ") {
			continue
		}

		filteredLines = append(filteredLines, line)
	}

	return strings.Join(filteredLines, "\n")
}

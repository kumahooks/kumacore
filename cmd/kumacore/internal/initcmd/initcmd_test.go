package initcmd

import (
	"bytes"
	"go/format"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateProjectName_UsesFunctionPrefix(t *testing.T) {
	err := validateProjectName(".")
	if err == nil {
		t.Fatal("validateProjectName: got nil error, want invalid project name")
	}

	if !strings.Contains(err.Error(), "[initcmd:validateProjectName]") {
		t.Fatalf("validateProjectName prefix: got %q", err.Error())
	}
}

func TestValidateModules_UsesFunctionPrefix(t *testing.T) {
	err := validateModules([]string{"home", "home"})
	if err == nil {
		t.Fatal("validateModules: got nil error, want duplicate module")
	}

	if !strings.Contains(err.Error(), "[initcmd:validateModules]") {
		t.Fatalf("validateModules prefix: got %q", err.Error())
	}
}

func TestReplaceMarkedRegion_RejectsInvalidMarkers(t *testing.T) {
	t.Run("missing begin marker", func(t *testing.T) {
		_, err := replaceMarkedRegion("before\n// end\n", "// begin", "// end", "content")
		if err == nil || !strings.Contains(err.Error(), "missing marker") {
			t.Fatalf("replaceMarkedRegion missing begin: got %v", err)
		}
	})

	t.Run("duplicate begin marker", func(t *testing.T) {
		source := "// begin\none\n// begin\ntwo\n// end\n"
		_, err := replaceMarkedRegion(source, "// begin", "// end", "content")
		if err == nil || !strings.Contains(err.Error(), "duplicate marker") {
			t.Fatalf("replaceMarkedRegion duplicate begin: got %v", err)
		}
	})

	t.Run("missing end marker", func(t *testing.T) {
		_, err := replaceMarkedRegion("// begin\nbody\n", "// begin", "// end", "content")
		if err == nil || !strings.Contains(err.Error(), "missing marker") {
			t.Fatalf("replaceMarkedRegion missing end: got %v", err)
		}
	})

	t.Run("duplicate end marker", func(t *testing.T) {
		source := "// begin\nbody\n// end\nsuffix\n// end\n"
		_, err := replaceMarkedRegion(source, "// begin", "// end", "content")
		if err == nil || !strings.Contains(err.Error(), "duplicate marker") {
			t.Fatalf("replaceMarkedRegion duplicate end: got %v", err)
		}
	})

	t.Run("invalid marker order", func(t *testing.T) {
		source := "// end\nbody\n// begin\n"
		_, err := replaceMarkedRegion(source, "// begin", "// end", "content")
		if err == nil || !strings.Contains(err.Error(), "marker order is invalid") {
			t.Fatalf("replaceMarkedRegion marker order: got %v", err)
		}
	})
}

func TestEnsureProjectDestination_RejectsExistingDirectory(t *testing.T) {
	targetRoot := filepath.Join(t.TempDir(), "existing-project")
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := ensureProjectDestination(targetRoot)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("ensureProjectDestination: got %v, want already exists error", err)
	}
}

func TestRewriteConfigDefaults_RewritesProjectSpecificValues(t *testing.T) {
	source := strings.Join([]string{
		`default:"./data/db/kumacore.db"`,
		`default:"./data/db/kumacore_worker.db"`,
		`default:"kumacore"`,
	}, "\n")

	rewrittenSource := rewriteConfigDefaults(source, "myapp")

	requiredSnippets := []string{
		`default:"./data/db/myapp.db"`,
		`default:"./data/db/myapp_worker.db"`,
		`default:"myapp"`,
	}

	for _, requiredSnippet := range requiredSnippets {
		if !strings.Contains(rewrittenSource, requiredSnippet) {
			t.Fatalf("rewriteConfigDefaults missing snippet %q", requiredSnippet)
		}
	}
}

func TestRenderGoMod_RewritesModulePath(t *testing.T) {
	t.Helper()

	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWorkingDirectory)
	})

	t.Chdir(repoRoot(t))

	rewrittenGoMod, err := renderGoMod("myapp")
	if err != nil {
		t.Fatalf("renderGoMod: %v", err)
	}

	if !strings.HasPrefix(string(rewrittenGoMod), "module myapp\n") {
		t.Fatalf("renderGoMod: unexpected module declaration\n%s", string(rewrittenGoMod))
	}
}

func TestRenderAppEmbed_MatchesRepositoryBaseline(t *testing.T) {
	t.Chdir(repoRoot(t))

	rendered, err := renderAppEmbed([]string{"home", "auth", "health"}, "kumacore")
	if err != nil {
		t.Fatalf("renderAppEmbed: %v", err)
	}

	baseline, err := os.ReadFile(appEmbedPath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", appEmbedPath, err)
	}

	if string(rendered) != string(baseline) {
		t.Fatalf(
			"renderAppEmbed output differs from %s:\n--- rendered ---\n%s\n--- baseline ---\n%s",
			appEmbedPath,
			rendered,
			baseline,
		)
	}
}

func TestRenderAppEmbed_OmitsModulesWithoutPageTemplates(t *testing.T) {
	t.Chdir(repoRoot(t))

	rendered, err := renderAppEmbed([]string{"health"}, "kumacore")
	if err != nil {
		t.Fatalf("renderAppEmbed: %v", err)
	}

	source := string(rendered)
	if strings.Contains(source, "modules/health/") {
		t.Fatalf("renderAppEmbed emitted a module pattern without page templates:\n%s", source)
	}

	for _, requiredPattern := range []string{
		"web/templates/layouts/*.html",
		"web/templates/components/*.html",
		"modules/errorscreen/*.html",
		"modules/*/components/*.html",
	} {
		if !strings.Contains(source, requiredPattern) {
			t.Fatalf("renderAppEmbed missing pattern %q:\n%s", requiredPattern, source)
		}
	}
}

func TestPlannedChanges_IncludesAppEmbed(t *testing.T) {
	t.Chdir(repoRoot(t))

	changes, err := plannedChanges(Options{ProjectName: "myapp", Modules: []string{"home"}})
	if err != nil {
		t.Fatalf("plannedChanges: %v", err)
	}

	var appEmbedChange *fileChange
	for index := range changes {
		if changes[index].path == appEmbedPath {
			appEmbedChange = &changes[index]
			break
		}
	}

	if appEmbedChange == nil {
		t.Fatalf("plannedChanges missing %s", appEmbedPath)
	}

	if !strings.Contains(string(appEmbedChange.data), "modules/home/*.html") {
		t.Fatalf("plannedChanges %s missing home module pattern:\n%s", appEmbedPath, appEmbedChange.data)
	}
}

func TestPlannedChanges_AlwaysIncludesErrorScreen(t *testing.T) {
	t.Chdir(repoRoot(t))

	changes, err := plannedChanges(Options{ProjectName: "myapp", Modules: []string{"health"}})
	if err != nil {
		t.Fatalf("plannedChanges: %v", err)
	}

	plannedPaths := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		plannedPaths[change.path] = struct{}{}
	}

	for _, requiredPath := range []string{
		"app/modules/errorscreen/errorscreen.html",
		"app/modules/errorscreen/components/error-body.html",
		"app/modules/errorscreen/handler.go",
	} {
		if _, exists := plannedPaths[requiredPath]; !exists {
			t.Fatalf("plannedChanges missing baseline error screen file %q", requiredPath)
		}
	}
}

func TestRewriteServerModules_AlwaysWiresErrorScreen(t *testing.T) {
	t.Chdir(repoRoot(t))

	source, err := os.ReadFile(serverModulesPath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", serverModulesPath, err)
	}

	rewritten, err := rewriteServerModules(
		source,
		metadataFromOptions(Options{Modules: []string{"health"}}),
		"myapp",
	)
	if err != nil {
		t.Fatalf("rewriteServerModules: %v", err)
	}

	for _, requiredSnippet := range []string{
		`"myapp/app/modules/errorscreen"`,
		`notFoundHandler := errorscreen.NewHandler(renderer)`,
		`errorscreen.Routes(notFoundHandler),`,
	} {
		if !strings.Contains(string(rewritten), requiredSnippet) {
			t.Fatalf("rewriteServerModules missing snippet %q:\n%s", requiredSnippet, rewritten)
		}
	}
}

func TestRewriteServerModules_AuthWiresBcrypt(t *testing.T) {
	t.Chdir(repoRoot(t))

	source, err := os.ReadFile(serverModulesPath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", serverModulesPath, err)
	}

	rewritten, err := rewriteServerModules(
		source,
		metadataFromOptions(Options{Modules: []string{"auth"}}),
		"myapp",
	)
	if err != nil {
		t.Fatalf("rewriteServerModules: %v", err)
	}

	for _, requiredSnippet := range []string{
		`"golang.org/x/crypto/bcrypt"`,
		`bcrypt.DefaultCost`,
	} {
		if !strings.Contains(string(rewritten), requiredSnippet) {
			t.Fatalf("rewriteServerModules missing auth snippet %q:\n%s", requiredSnippet, rewritten)
		}
	}
}

func TestRewriteServerModules_ImportsAreGofmtFormatted(t *testing.T) {
	t.Chdir(repoRoot(t))

	source, err := os.ReadFile(serverModulesPath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", serverModulesPath, err)
	}

	for _, modules := range [][]string{
		{"auth"},
		{"auth", "health"},
		{"home", "auth", "health"},
	} {
		rewritten, err := rewriteServerModules(source, metadataFromOptions(Options{Modules: modules}), "myapp")
		if err != nil {
			t.Fatalf("rewriteServerModules(%v): %v", modules, err)
		}

		formatted, err := format.Source(rewritten)
		if err != nil {
			t.Fatalf("rewriteServerModules(%v) produced invalid Go: %v", modules, err)
		}

		if !bytes.Equal(formatted, rewritten) {
			t.Fatalf("rewriteServerModules(%v) is not gofmt-formatted:\n%s", modules, rewritten)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, currentFilePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: unable to resolve current file path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(currentFilePath), "..", "..", "..", ".."))
}

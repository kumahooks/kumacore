package initcmd

import (
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

func repoRoot(t *testing.T) string {
	t.Helper()

	_, currentFilePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: unable to resolve current file path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(currentFilePath), "..", "..", "..", ".."))
}

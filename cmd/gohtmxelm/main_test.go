package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapTemplatePath(t *testing.T) {
	cases := map[string]string{
		"main.go.tmpl":         "main.go",
		"page.templ.tmpl":      "page.templ",
		"gitignore.tmpl":       ".gitignore",
		"elm/Counter.elm.tmpl": filepath.Join("elm", "Counter.elm"),
		"static/.gitkeep":      filepath.Join("static", ".gitkeep"),
	}
	for in, want := range cases {
		if got := mapTemplatePath(in); got != want {
			t.Errorf("mapTemplatePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeModule(t *testing.T) {
	cases := map[string]string{
		"my app":  "my-app",
		"":        "app",
		".":       "app",
		"counter": "counter",
	}
	for in, want := range cases {
		if got := sanitizeModule(in); got != want {
			t.Errorf("sanitizeModule(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestModulePath(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module example.com/host\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := modulePath(goMod); got != "example.com/host" {
		t.Errorf("modulePath = %q, want example.com/host", got)
	}
	if got := modulePath(filepath.Join(dir, "missing.mod")); got != "" {
		t.Errorf("modulePath(missing) = %q, want empty", got)
	}
}

// writeTree must materialise every scaffold flavour with destination names
// mapped (.tmpl stripped, gitignore restored) and package declarations intact.
func TestWriteTreeFlavours(t *testing.T) {
	cases := []struct {
		set      string
		wantFile string
		wantPkg  string
	}{
		{"templates/new", "main.go", "package main"},
		{"templates/new", "page.templ", "package main"},
		{"templates/minimal", "main.go", "package main"},
		{"templates/existing", "kit.go", "package gohtmxelmkit"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		if err := writeTree(c.set, dir, false); err != nil {
			t.Fatalf("writeTree(%s): %v", c.set, err)
		}
		data, err := os.ReadFile(filepath.Join(dir, c.wantFile))
		if err != nil {
			t.Fatalf("%s/%s not written: %v", c.set, c.wantFile, err)
		}
		if !strings.Contains(string(data), c.wantPkg) {
			t.Errorf("%s/%s missing %q", c.set, c.wantFile, c.wantPkg)
		}
		// .tmpl artefacts must never leak into the output tree.
		_ = filepath.WalkDir(dir, func(path string, _ os.DirEntry, _ error) error {
			if strings.HasSuffix(path, ".tmpl") {
				t.Errorf("leaked template file %s", path)
			}
			return nil
		})
	}
}

func TestEmitDeploy(t *testing.T) {
	cases := []struct {
		minimal     bool
		mustContain string // in the Dockerfile
		mustAbsent  string
	}{
		{false, "templ generate", ""}, // full builds templ + copies static
		{true, "", "templ generate"},  // minimal is Go-only
	}
	want := []string{"Dockerfile", ".dockerignore", "docker-compose.yml", "DEPLOY.md", filepath.Join(".github", "workflows", "deploy.yml")}
	for _, c := range cases {
		dir := t.TempDir()
		if err := emitDeploy(dir, c.minimal, false); err != nil {
			t.Fatalf("emitDeploy(minimal=%v): %v", c.minimal, err)
		}
		for _, f := range want {
			if !fileExists(filepath.Join(dir, f)) {
				t.Errorf("minimal=%v: missing %s", c.minimal, f)
			}
		}
		df, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
		if err != nil {
			t.Fatal(err)
		}
		if c.mustContain != "" && !strings.Contains(string(df), c.mustContain) {
			t.Errorf("minimal=%v: Dockerfile missing %q", c.minimal, c.mustContain)
		}
		if c.mustAbsent != "" && strings.Contains(string(df), c.mustAbsent) {
			t.Errorf("minimal=%v: Dockerfile should not contain %q", c.minimal, c.mustAbsent)
		}
	}
}

func TestWriteFileNoClobber(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "go.mod")
	if err := writeFile(p, "first", false); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(p, "second", false); err == nil {
		t.Error("expected error overwriting without --force")
	}
	if err := writeFile(p, "second", true); err != nil {
		t.Errorf("force overwrite failed: %v", err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "second" {
		t.Errorf("force overwrite content = %q, want second", data)
	}
}

// Command gohtmxelm scaffolds and inspects gohtmxelm projects.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nkhine/gohtmxelm"
)

//go:embed all:templates
var templatesFS embed.FS

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage()
		return nil
	case "init":
		return initCmd(args[1:])
	case "vendor-elm":
		return vendorElmCmd(args[1:])
	case "doctor":
		return doctor()
	default:
		return fmt.Errorf("unknown command %q (try `gohtmxelm help`)", args[0])
	}
}

func usage() {
	fmt.Println(`gohtmxelm wires Go, HTMX, Datastar, Elm, and SSE.

Usage:
  gohtmxelm init [dir] [flags]   scaffold a project (or integrate into an existing one)
  gohtmxelm vendor-elm [dir]     (re)write BrokerPort.elm to keep the Elm contract in sync
  gohtmxelm doctor               check the local toolchain

init flags:
  --module <path>   module path for a new project (default: target directory name)
  --minimal         scaffold an SSE-only example (no Elm, no build step)
  --no-build        only write files; skip go get / templ generate / elm make
  --force           overwrite existing files

Run inside a directory that already has a go.mod and init adds a mountable
gohtmxelmkit/ package instead of a standalone app.`)
}

// ---- init ----

type initOptions struct {
	module  string
	minimal bool
	noBuild bool
	force   bool
}

func initCmd(args []string) error {
	fset := flag.NewFlagSet("init", flag.ContinueOnError)
	var opts initOptions
	fset.StringVar(&opts.module, "module", "", "module path for a new project")
	fset.BoolVar(&opts.minimal, "minimal", false, "scaffold an SSE-only example")
	fset.BoolVar(&opts.noBuild, "no-build", false, "only write files")
	fset.BoolVar(&opts.force, "force", false, "overwrite existing files")
	// Parse twice so flags may appear before or after the positional dir: the
	// std flag package stops at the first non-flag argument, so we consume the
	// leading flags, lift out dir, then parse any trailing flags.
	if err := fset.Parse(args); err != nil {
		return err
	}
	dir := "."
	if rest := fset.Args(); len(rest) > 0 {
		dir = rest[0]
		if err := fset.Parse(rest[1:]); err != nil {
			return err
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}

	if fileExists(filepath.Join(abs, "go.mod")) {
		if opts.minimal {
			return fmt.Errorf("--minimal applies to new projects; this directory already has a go.mod")
		}
		return initExisting(abs, opts)
	}
	return initNew(abs, opts)
}

func initNew(dir string, opts initOptions) error {
	module := opts.module
	if module == "" {
		module = sanitizeModule(filepath.Base(dir))
	}

	set := "new"
	if opts.minimal {
		set = "minimal"
	}
	if err := writeTree(filepath.Join("templates", set), dir, opts.force); err != nil {
		return err
	}
	if !opts.minimal {
		if err := vendorElm(filepath.Join(dir, "elm"), opts.force); err != nil {
			return err
		}
	}
	if err := writeFile(filepath.Join(dir, "README.md"), newReadme(module, opts.minimal), opts.force); err != nil {
		return err
	}
	// Write go.mod directly (offline) so the directory is a valid module even
	// under --no-build; `go mod tidy` later fills in requirements and sums.
	if err := writeFile(filepath.Join(dir, "go.mod"), goModFile(module), opts.force); err != nil {
		return err
	}

	fmt.Printf("scaffolded %s in %s\n", describe(opts.minimal), dir)

	if opts.noBuild {
		printNextSteps(dir, module, opts.minimal, true)
		return nil
	}

	// Bring the module to a buildable state. Each step is best-effort: a failure
	// (commonly: offline) is reported and the equivalent manual command printed.
	steps := buildSteps(dir, opts.minimal)
	failed := runSteps(dir, steps)
	printNextSteps(dir, module, opts.minimal, failed)
	return nil
}

func initExisting(dir string, opts initOptions) error {
	module := modulePath(filepath.Join(dir, "go.mod"))
	target := filepath.Join(dir, "gohtmxelmkit")
	if err := writeTree(filepath.Join("templates", "existing"), target, opts.force); err != nil {
		return err
	}
	if err := vendorElm(filepath.Join(target, "elm"), opts.force); err != nil {
		return err
	}

	fmt.Printf("added gohtmxelmkit/ to the existing module %q\n", module)
	fmt.Println()
	importPath := "gohtmxelmkit"
	if module != "" {
		importPath = module + "/gohtmxelmkit"
	}
	fmt.Println("Wire it into your chi router:")
	fmt.Printf("\n    import \"%s\"\n\n", importPath)
	fmt.Println("    kit := gohtmxelmkit.New(\"/counter\") // any prefix, or \"\" for root")
	fmt.Println("    kit.Mount(r)                        // r is your chi.Router")
	fmt.Println()
	fmt.Println("Then build the assets and run your server:")
	fmt.Println("\n    go get github.com/nkhine/gohtmxelm@latest github.com/go-chi/chi/v5")
	fmt.Println("    go get -tool github.com/a-h/templ/cmd/templ")
	fmt.Println("    make -C gohtmxelmkit build")
	fmt.Println("    go run .   # then open /counter")
	fmt.Println()
	fmt.Println("Note: gohtmxelmkit serves its Elm bundle from ./gohtmxelmkit/static")
	fmt.Println("relative to the working directory; run from your module root.")
	return nil
}

// ---- vendor-elm ----

func vendorElmCmd(args []string) error {
	dir := "elm"
	if len(args) > 0 {
		dir = args[0]
	}
	if err := vendorElm(dir, true); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", filepath.Join(dir, "BrokerPort.elm"))
	return nil
}

// vendorElm writes the canonical BrokerPort.elm contract into dir.
func vendorElm(dir string, force bool) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, "BrokerPort.elm"), gohtmxelm.ElmBrokerPort(), force)
}

// ---- build orchestration ----

type step struct {
	desc    string
	dir     string
	name    string
	args    []string
	manual  string
	skipErr bool // tool-missing is fine (e.g. elm not installed)
}

func buildSteps(dir string, minimal bool) []step {
	steps := []step{
		{desc: "go get gohtmxelm", dir: dir, name: "go", args: []string{"get", "github.com/nkhine/gohtmxelm@latest"}, manual: "go get github.com/nkhine/gohtmxelm@latest"},
		{desc: "go get chi", dir: dir, name: "go", args: []string{"get", "github.com/go-chi/chi/v5"}, manual: "go get github.com/go-chi/chi/v5"},
	}
	if !minimal {
		steps = append(steps,
			step{desc: "go get -tool templ", dir: dir, name: "go", args: []string{"get", "-tool", "github.com/a-h/templ/cmd/templ"}, manual: "go get -tool github.com/a-h/templ/cmd/templ"},
		)
	}
	steps = append(steps, step{desc: "go mod tidy", dir: dir, name: "go", args: []string{"mod", "tidy"}, manual: "go mod tidy"})
	if !minimal {
		steps = append(steps,
			step{desc: "templ generate", dir: dir, name: "go", args: []string{"tool", "templ", "generate"}, manual: "go tool templ generate"},
			step{desc: "elm make", dir: dir, name: "elm", args: []string{"make", "elm/Counter.elm", "--output=static/elm.js"}, manual: "elm make elm/Counter.elm --output=static/elm.js", skipErr: true},
		)
	}
	return steps
}

// runSteps executes steps in order, stopping the build chain on the first hard
// failure. It returns true if any step failed (so the caller prints manual
// fallbacks).
func runSteps(dir string, steps []step) bool {
	failed := false
	for _, s := range steps {
		if s.name == "elm" && !hasTool("elm") {
			fmt.Printf("  skip   %-18s (elm not installed)\n", s.desc)
			failed = true
			continue
		}
		fmt.Printf("  run    %s\n", s.desc)
		cmd := exec.Command(s.name, s.args...)
		cmd.Dir = s.dir
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("  fail   %s: %v\n", s.desc, err)
			failed = true
			if !s.skipErr {
				// Later steps depend on this one; stop the chain.
				return failed
			}
		}
	}
	return failed
}

func printNextSteps(dir, module string, minimal, needManual bool) {
	rel := relOrDir(dir)
	fmt.Println()
	if needManual {
		fmt.Println("Finish setup manually:")
		fmt.Printf("\n    cd %s\n", rel)
		if minimal {
			fmt.Println("    go mod tidy")
			fmt.Println("    go run .")
		} else {
			fmt.Println("    go get github.com/nkhine/gohtmxelm@latest github.com/go-chi/chi/v5")
			fmt.Println("    go get -tool github.com/a-h/templ/cmd/templ")
			fmt.Println("    make dev   # tidy, templ generate, elm make, go run")
		}
		fmt.Println()
		return
	}
	fmt.Println("Ready. Start it with:")
	if minimal {
		fmt.Printf("\n    cd %s && go run .\n\n", rel)
	} else {
		fmt.Printf("\n    cd %s && make dev\n\n", rel)
	}
	fmt.Println("Then open http://localhost:8080")
}

// ---- file helpers ----

// writeTree copies every embedded template file under src into dst, stripping
// the .tmpl suffix and restoring dotfiles (gitignore.tmpl -> .gitignore).
func writeTree(src, dst string, force bool) error {
	return fs.WalkDir(templatesFS, src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		data, err := templatesFS.ReadFile(path)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, mapTemplatePath(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return writeFile(out, string(data), force)
	})
}

// mapTemplatePath turns a template-relative path into its destination name.
func mapTemplatePath(rel string) string {
	dir, base := filepath.Split(rel)
	switch {
	case base == "gitignore.tmpl":
		base = ".gitignore"
	case strings.HasSuffix(base, ".tmpl"):
		base = strings.TrimSuffix(base, ".tmpl")
	}
	return filepath.Join(dir, base)
}

func writeFile(path, content string, force bool) error {
	if !force && fileExists(path) {
		return fmt.Errorf("%s already exists (use --force to overwrite)", path)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasTool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// sanitizeModule makes a directory name usable as a Go module path.
func sanitizeModule(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, " ", "-")
	if name == "" || name == "." || name == "/" {
		return "app"
	}
	return name
}

// modulePath reads the module path from a go.mod file ("" if unreadable).
func modulePath(goMod string) string {
	data, err := os.ReadFile(goMod)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func relOrDir(dir string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, dir); err == nil && !strings.HasPrefix(rel, "..") {
			if rel == "." {
				return "."
			}
			return rel
		}
	}
	return dir
}

func goModFile(module string) string {
	return "module " + module + "\n\ngo 1.25\n"
}

func describe(minimal bool) string {
	if minimal {
		return "a minimal SSE-only app"
	}
	return "an Elm-island app"
}

func newReadme(module string, minimal bool) string {
	if minimal {
		return "# " + module + `

A minimal [gohtmxelm](https://github.com/nkhine/gohtmxelm) app: a server-owned
counter pushed to the browser over SSE with a plain EventSource client — no Elm
and no build step.

## Run

    make dev        # tidy deps and run on :8080 (override with PORT=...)

Open http://localhost:8080 and click **+1**. Go owns the count; every open tab
re-renders from the SSE stream.

## Layout

    main.go         chi server: the SSE stream, /api/bump, and the page

Upgrade to the full Elm-island scaffold any time with ` + "`gohtmxelm init`" + ` in a
fresh directory.
`
	}
	return "# " + module + `

A starter [gohtmxelm](https://github.com/nkhine/gohtmxelm) app: a server-owned
counter pushed to an Elm island over SSE.

## Run

    make dev        # tidy deps, generate templ, build Elm, run on :8080

Open http://localhost:8080 and click **+1**. Go owns the count; every open tab
re-renders from the SSE stream.

## Layout

    main.go             chi server: mounts the broker runtime, the SSE stream, /api/bump
    page.templ          host shell (templ); injects the broker script + Elm island
    elm/Counter.elm     the Elm island
    elm/BrokerPort.elm  canonical wire contract (vendored from the library)
    static/elm.js       compiled Elm bundle (built by ` + "`make elm`" + `)

After upgrading gohtmxelm, re-vendor the Elm contract so it matches the broker:

    go run github.com/nkhine/gohtmxelm/cmd/gohtmxelm vendor-elm
`
}

// ---- doctor ----

func doctor() error {
	type tool struct {
		name     string
		required bool
		note     string
	}
	tools := []tool{
		{"go", true, "the compiler and module tooling"},
		{"elm", false, "Elm island compiler (optional; full scaffold)"},
		{"templ", false, "optional: the full scaffold uses `go tool templ` instead"},
		{"air", false, "optional: live reload"},
	}
	var missingRequired []string
	for _, t := range tools {
		path, err := exec.LookPath(t.name)
		if err != nil {
			tag := "optional"
			if t.required {
				tag = "MISSING "
				missingRequired = append(missingRequired, t.name)
			}
			fmt.Printf("%-9s %-6s %s\n", tag, t.name, t.note)
			continue
		}
		fmt.Printf("found     %-6s %s\n", t.name, path)
	}
	if len(missingRequired) > 0 {
		return fmt.Errorf("missing required tools: %s", strings.Join(missingRequired, ", "))
	}
	return nil
}

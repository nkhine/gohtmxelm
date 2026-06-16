// Command gohtmxelm scaffolds and inspects gohtmxelm projects.
package main

import (
	"bytes"
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
		if len(args) > 1 {
			return helpFor(args[1])
		}
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

func helpFor(cmd string) error {
	switch cmd {
	case "init":
		helpInit()
	case "vendor-elm", "doctor", "help":
		usage()
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
	return nil
}

func usage() {
	h := func(s string) string { return bold(s) }
	fmt.Printf(`
  %s — wire Go, HTMX, Datastar, Elm, and SSE

  %s
    gohtmxelm <command> [dir] [flags]

  %s
    %s   scaffold a runnable project, or integrate into an existing module
    %s   (re)write BrokerPort.elm to keep the Elm contract in sync
    %s   check the local toolchain
    %s   show help for a command (e.g. gohtmxelm help init)

  %s
    --module <path>   module path for a new project (default: directory name)
    --minimal         SSE-only example: no Elm, no build step
    --no-build        write files only; skip go get / templ generate / elm make
    --force           overwrite existing files

  %s
    gohtmxelm init myapp              new Elm-island app in ./myapp
    gohtmxelm init myapp --minimal    SSE-only, runs with `+"`go run .`"+`
    gohtmxelm init                    add gohtmxelmkit/ to the current module
    gohtmxelm vendor-elm elm          refresh elm/BrokerPort.elm

`,
		cyan(bold("◆ gohtmxelm")),
		h("USAGE"), h("COMMANDS"),
		bold("init      "), bold("vendor-elm"), bold("doctor    "), bold("help      "),
		h("INIT FLAGS"), h("EXAMPLES"),
	)
}

func helpInit() {
	h := func(s string) string { return bold(s) }
	fmt.Printf(`
  %s — scaffold a gohtmxelm project

  %s
    gohtmxelm init [dir] [flags]

  In an empty directory, init generates a complete, runnable example: a chi +
  templ server, an SSE-backed Broadcaster, and a sample Elm island wired through
  the vendored BrokerPort contract — then installs deps and builds the assets.

  Run it where a %s already exists and it instead drops a self-contained,
  mountable %s package and prints the chi wiring snippet, never
  touching your existing code.

  %s
    --module <path>   module path for a new project (default: directory name)
    --minimal         SSE-only example: no Elm, no build step
    --no-build        write files only; skip go get / templ generate / elm make
    --force           overwrite existing files

`,
		cyan(bold("◆ gohtmxelm init")), h("USAGE"),
		bold("go.mod"), bold("gohtmxelmkit/"), h("FLAGS"),
	)
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
	fset.Usage = helpInit
	var opts initOptions
	fset.StringVar(&opts.module, "module", "", "module path for a new project")
	fset.BoolVar(&opts.minimal, "minimal", false, "scaffold an SSE-only example")
	fset.BoolVar(&opts.noBuild, "no-build", false, "only write files")
	fset.BoolVar(&opts.force, "force", false, "overwrite existing files")
	// Parse twice so flags may appear before or after the positional dir: the
	// std flag package stops at the first non-flag argument, so we consume the
	// leading flags, lift out dir, then parse any trailing flags.
	if err := fset.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	dir := "."
	if rest := fset.Args(); len(rest) > 0 {
		dir = rest[0]
		if err := fset.Parse(rest[1:]); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
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
	kind := "Elm-island app"
	set := "new"
	if opts.minimal {
		kind, set = "SSE-only app", "minimal"
	}
	banner(fmt.Sprintf("new %s  %s  %s", kind, cyan("→"), bold(relOrDir(dir))))

	phase("Creating files")
	if err := step("project files", func() (string, error) {
		if err := writeTree(filepath.Join("templates", set), dir, opts.force); err != nil {
			return "", err
		}
		if !opts.minimal {
			if err := vendorElm(filepath.Join(dir, "elm"), opts.force); err != nil {
				return "", err
			}
		}
		if err := writeFile(filepath.Join(dir, "README.md"), newReadme(module, opts.minimal), opts.force); err != nil {
			return "", err
		}
		// Write go.mod directly (offline) so the directory is a valid module even
		// under --no-build; `go mod tidy` later fills in requirements and sums.
		return "", writeFile(filepath.Join(dir, "go.mod"), goModFile(module), opts.force)
	}); err != nil {
		return err
	}

	if opts.noBuild {
		printNextSteps(dir, opts.minimal, true)
		return nil
	}

	// Bring the module to a buildable state. Each task is best-effort: a failure
	// (commonly: offline) is reported and the equivalent manual command printed.
	failed := runTasks(dir, buildTasks(dir, opts.minimal))
	printNextSteps(dir, opts.minimal, failed)
	return nil
}

func initExisting(dir string, opts initOptions) error {
	module := modulePath(filepath.Join(dir, "go.mod"))
	moduleLabel := module
	if moduleLabel == "" {
		moduleLabel = "this module"
	}
	target := filepath.Join(dir, "gohtmxelmkit")
	banner(fmt.Sprintf("integrate into  %s", bold(moduleLabel)))

	phase("Creating files")
	if err := step("gohtmxelmkit/ package", func() (string, error) {
		if err := writeTree(filepath.Join("templates", "existing"), target, opts.force); err != nil {
			return "", err
		}
		return "", vendorElm(filepath.Join(target, "elm"), opts.force)
	}); err != nil {
		return err
	}

	importPath := "gohtmxelmkit"
	if module != "" {
		importPath = module + "/gohtmxelmkit"
	}
	fmt.Println()
	phase("Wire it into your chi router")
	fmt.Printf("    import %q\n\n", importPath)
	fmt.Println("    kit := gohtmxelmkit.New(\"/counter\") // any prefix, or \"\" for root")
	fmt.Println("    kit.Mount(r)                        // r is your chi.Router")
	fmt.Println()
	phase("Then build and run")
	fmt.Println("    go get github.com/nkhine/gohtmxelm@latest github.com/go-chi/chi/v5")
	fmt.Println("    go get -tool github.com/a-h/templ/cmd/templ")
	fmt.Println("    make -C gohtmxelmkit build")
	fmt.Printf("    go run .   %s\n\n", dim("# then open /counter"))
	fmt.Printf("  %s the Elm bundle is served from ./gohtmxelmkit/static relative to the\n", dim("note:"))
	fmt.Printf("        working directory — run from your module root.\n\n")
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
	ok(fmt.Sprintf("wrote %s", filepath.Join(dir, "BrokerPort.elm")))
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

type task struct {
	phase    string
	label    string
	name     string
	args     []string
	optional bool // a missing tool (e.g. elm) is reported, not fatal
}

func buildTasks(dir string, minimal bool) []task {
	const deps, assets = "Installing dependencies", "Building assets"
	tasks := []task{
		{deps, "gohtmxelm", "go", []string{"get", "github.com/nkhine/gohtmxelm@latest"}, false},
		{deps, "chi router", "go", []string{"get", "github.com/go-chi/chi/v5"}, false},
	}
	if !minimal {
		tasks = append(tasks, task{deps, "templ tool", "go", []string{"get", "-tool", "github.com/a-h/templ/cmd/templ"}, false})
	}
	tasks = append(tasks, task{deps, "resolving modules", "go", []string{"mod", "tidy"}, false})
	if !minimal {
		tasks = append(tasks,
			task{assets, "generating templ components", "go", []string{"tool", "templ", "generate"}, false},
			task{assets, "compiling Elm bundle", "elm", []string{"make", "elm/Counter.elm", "--output=static/elm.js"}, true},
		)
	}
	return tasks
}

// runTasks runs each task under a spinner, printing a phase header when it
// changes. It stops the chain on the first hard failure (later tasks depend on
// earlier ones) and returns true if anything failed or was skipped.
func runTasks(dir string, tasks []task) bool {
	failed := false
	current := ""
	for _, t := range tasks {
		if t.phase != current {
			current = t.phase
			fmt.Println()
			phase(current)
		}
		if t.name == "elm" && !hasTool("elm") {
			skipped(t.label, "elm not installed")
			failed = true
			continue
		}
		err := step(t.label, func() (string, error) { return runCmd(dir, t.name, t.args) })
		if err != nil {
			failed = true
			if !t.optional {
				return failed
			}
		}
	}
	return failed
}

// runCmd runs a command in dir, capturing combined output so the spinner stays
// clean; the output is surfaced only when the command fails.
func runCmd(dir, name string, args []string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	return buf.String(), cmd.Run()
}

func printNextSteps(dir string, minimal, needManual bool) {
	rel := relOrDir(dir)
	fmt.Println()
	if needManual {
		phase("Finish setup")
		if minimal {
			fmt.Printf("    cd %s && go run .\n\n", rel)
			return
		}
		fmt.Printf("    cd %s\n", rel)
		fmt.Println("    go get github.com/nkhine/gohtmxelm@latest github.com/go-chi/chi/v5")
		fmt.Println("    go get -tool github.com/a-h/templ/cmd/templ")
		fmt.Printf("    make dev\n\n")
		return
	}
	startCmd := "make dev"
	if minimal {
		startCmd = "go run ."
	}
	fmt.Printf("  %s %s\n", green("✔"), bold("Ready"))
	fmt.Printf("    %s cd %s && %s\n", cyan("→"), rel, startCmd)
	fmt.Printf("    %s open http://localhost:8080\n\n", cyan("→"))
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
	banner("toolchain check")
	phase("Tools")
	type tool struct {
		name     string
		required bool
		note     string
	}
	tools := []tool{
		{"go", true, "compiler and module tooling"},
		{"elm", false, "Elm island compiler (full scaffold)"},
		{"templ", false, "optional — the scaffold uses `go tool templ`"},
		{"air", false, "optional — live reload"},
	}
	var missingRequired []string
	for _, t := range tools {
		path, err := exec.LookPath(t.name)
		if err != nil {
			if t.required {
				missingRequired = append(missingRequired, t.name)
				fail(fmt.Sprintf("%-6s %s", t.name, t.note), "")
			} else {
				skipped(fmt.Sprintf("%-6s", t.name), t.note)
			}
			continue
		}
		ok(fmt.Sprintf("%-6s %s", t.name, dim(path)))
	}
	fmt.Println()
	if len(missingRequired) > 0 {
		return fmt.Errorf("missing required tools: %s", strings.Join(missingRequired, ", "))
	}
	return nil
}

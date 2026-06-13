package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const configTemplate = `# gohtmxelm integration config

[assets]
path = "/gohtmxelm"

[elm]
src = "elm"
out = "static/elm"

[events]
stream = "/api/events"
names = ["store-change"]
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
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
		return initProject()
	case "doctor":
		return doctor()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Println(`gohtmxelm wires Go, HTMX, Datastar, Elm, and SSE.

Usage:
  gohtmxelm init      create gohtmxelm.toml
  gohtmxelm doctor    check local toolchain`)
}

func initProject() error {
	const filename = "gohtmxelm.toml"
	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("%s already exists", filename)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(filename, []byte(configTemplate), 0o644); err != nil {
		return err
	}
	fmt.Println("created gohtmxelm.toml")
	fmt.Println("import pkg as: gohtmxelm \"github.com/nkhine/gohtmxelm/pkg\"")
	fmt.Println("mount assets with: http.StripPrefix(\"/gohtmxelm/\", gohtmxelm.Assets())")
	return nil
}

func doctor() error {
	tools := []string{"go", "elm", "templ", "air"}
	var missing []string
	for _, tool := range tools {
		path, err := exec.LookPath(tool)
		if err != nil {
			fmt.Printf("missing  %s\n", tool)
			missing = append(missing, tool)
			continue
		}
		fmt.Printf("found    %-6s %s\n", tool, path)
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing tools: %s", strings.Join(missing, ", "))
	}
	return nil
}

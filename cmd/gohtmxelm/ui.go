package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Terminal UI helpers: phase headers, status icons, and an animated spinner for
// long-running steps. Everything degrades to plain, ANSI-free lines when stdout
// is not a terminal (pipes, CI, logs) or NO_COLOR is set, so captured output
// stays clean.

var (
	interactive  = isTTY()
	colorEnabled = interactive && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// detailOr falls back to the error message when no captured output is available.
func detailOr(detail string, err error) string {
	if strings.TrimSpace(detail) == "" && err != nil {
		return err.Error()
	}
	return detail
}

func paint(code, s string) string {
	if !colorEnabled {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func bold(s string) string  { return paint("1", s) }
func dim(s string) string   { return paint("2", s) }
func green(s string) string { return paint("32", s) }
func red(s string) string   { return paint("31", s) }
func cyan(s string) string  { return paint("36", s) }
func amber(s string) string { return paint("33", s) }

// banner prints the title block at the top of a command.
func banner(line string) {
	fmt.Printf("\n  %s %s\n\n", cyan(bold("◆ gohtmxelm")), line)
}

// phase prints a section header.
func phase(title string) {
	fmt.Printf("  %s\n", bold(title))
}

// note prints an indented ✓ line for an already-completed fact.
func ok(label string) {
	fmt.Printf("    %s %s\n", green("✓"), label)
}

// skipped prints an amber line for a step that was intentionally not run.
func skipped(label, why string) {
	fmt.Printf("    %s %s %s\n", amber("‒"), label, dim("("+why+")"))
}

// fail prints a red ✗ line and the captured detail, indented.
func fail(label, detail string) {
	fmt.Printf("    %s %s\n", red("✗"), label)
	if detail = strings.TrimSpace(detail); detail != "" {
		for _, l := range strings.Split(detail, "\n") {
			fmt.Printf("      %s\n", dim(l))
		}
	}
}

// step executes fn while showing an animated spinner next to label, then prints
// ✓ on success or ✗ plus the returned detail on failure. Non-interactive
// terminals get a single plain status line.
func step(label string, fn func() (detail string, err error)) error {
	if !interactive {
		detail, err := fn()
		if err != nil {
			fail(label, detailOr(detail, err))
			return err
		}
		ok(label)
		return nil
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			case <-t.C:
				fmt.Printf("\r    %s %s", cyan(spinnerFrames[i%len(spinnerFrames)]), label)
			}
		}
	}()

	detail, err := fn()
	close(stop)
	wg.Wait()
	fmt.Print("\r\033[K") // return to start of line and clear it

	if err != nil {
		fail(label, detail)
		return err
	}
	ok(label)
	return nil
}

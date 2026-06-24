package gohtmxelm

import (
	"encoding/json"
	htmlpkg "html"
	"strconv"
	"strings"
)

// CanvasOptions configures a single immediate-mode canvas island. The library
// owns mounting, input normalisation, resize handling, SSE delivery, and command
// posting; the host JavaScript module owns the actual drawing and tool policy.
type CanvasOptions struct {
	// Width and Height set optional CSS dimensions in pixels. The browser
	// runtime still sizes the backing buffer for devicePixelRatio.
	Width  int
	Height int
	// CommandURL receives JSON commands emitted by the island runtime with
	// api.command(payload). Leave empty for read-only visualisations.
	CommandURL string
	// Events limits which SSE event names are delivered to this canvas. Empty
	// means every configured/brokered SSE event is delivered.
	Events []string
	// Class appends CSS classes to the default "gohtmxelm-imui" class.
	Class string
	// Role defaults to "img" when Label is set.
	Role string
	// Label becomes aria-label and should name the canvas for assistive tech.
	Label string
}

// IMUIScript returns a <script> tag that loads the embedded immediate-mode
// canvas runtime. It shares Options.Sources with BrowserScript so IMUI can work
// both beside the broker and on pages with no Elm islands.
func IMUIScript(opts Options) string {
	if opts.AssetPath == "" {
		opts.AssetPath = "/gohtmxelm"
	}

	var attrs strings.Builder
	attrs.WriteString(` defer src="`)
	attrs.WriteString(htmlpkg.EscapeString(strings.TrimRight(opts.AssetPath, "/")))
	attrs.WriteString(`/gohtmxelm-imui.js"`)
	if len(opts.Sources) > 0 {
		if b, err := json.Marshal(opts.Sources); err == nil {
			attrs.WriteString(` data-sources="`)
			attrs.WriteString(htmlpkg.EscapeString(string(b)))
			attrs.WriteString(`"`)
		}
	}
	if opts.Debug {
		attrs.WriteString(` data-debug="true"`)
	}
	return "<script" + attrs.String() + "></script>"
}

// CanvasIsland returns the canvas mount point convention used by the IMUI
// runtime. The module is resolved from window.GoHTMXElmIMUI.register(name, ...).
func CanvasIsland(id string, module string, props any, opts CanvasOptions) (string, error) {
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", err
	}
	eventsJSON, err := json.Marshal(opts.Events)
	if err != nil {
		return "", err
	}

	class := strings.TrimSpace("gohtmxelm-imui " + opts.Class)
	attrs := [][2]string{
		{"class", class},
		{"id", id},
		{"data-gohtmxelm-imui-module", module},
		{"data-gohtmxelm-imui-id", id},
		{"data-props", string(propsJSON)},
		{"tabindex", "0"},
	}
	if opts.CommandURL != "" {
		attrs = append(attrs, [2]string{"data-command-url", opts.CommandURL})
	}
	if len(opts.Events) > 0 {
		attrs = append(attrs, [2]string{"data-events", string(eventsJSON)})
	}
	if opts.Label != "" {
		role := opts.Role
		if role == "" {
			role = "img"
		}
		attrs = append(attrs, [2]string{"role", role}, [2]string{"aria-label", opts.Label})
	} else if opts.Role != "" {
		attrs = append(attrs, [2]string{"role", opts.Role})
	}
	if opts.Width > 0 {
		attrs = append(attrs, [2]string{"width", strconv.Itoa(opts.Width)})
	}
	if opts.Height > 0 {
		attrs = append(attrs, [2]string{"height", strconv.Itoa(opts.Height)})
	}

	var b strings.Builder
	b.WriteString("<canvas")
	b.WriteString(attrsString(attrs))
	b.WriteString("></canvas>")
	return b.String(), nil
}

// IMUICommandEvent is the DOM CustomEvent detail emitted as gohtmxelm:imui-command
// before the runtime posts a command to Go.
type IMUICommandEvent struct {
	IslandID string `json:"islandId"`
	Command  any    `json:"command"`
}

// MarshalIMUICommandEvent is a small testable helper for hosts that want to
// emit the same event shape from custom JavaScript or SSE payloads.
func MarshalIMUICommandEvent(islandID string, command any) (string, error) {
	b, err := json.Marshal(IMUICommandEvent{IslandID: islandID, Command: command})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

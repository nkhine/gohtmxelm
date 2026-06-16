package gohtmxelm

import (
	"encoding/json"
	htmlpkg "html"
	"strings"
)

// InteractionRoot returns the shared overlay root used by the browser
// interaction runtime. Server-rendered fragments can be appended here via
// data-gohtmxelm-open or GoHTMXElmInteractions.open().
func InteractionRoot(id string) string {
	if id == "" {
		id = "gohtmxelm-interactions"
	}
	escaped := htmlpkg.EscapeString(id)
	return `<div id="` + escaped + `" data-gohtmxelm-interactions-root></div>`
}

// InteractionScript returns a <script> tag for the embedded interaction
// runtime. Mount Assets under the same AssetPath used for BrowserScript.
func InteractionScript(opts Options) string {
	if opts.AssetPath == "" {
		opts.AssetPath = "/gohtmxelm"
	}
	var attrs strings.Builder
	attrs.WriteString(` defer src="`)
	attrs.WriteString(htmlpkg.EscapeString(strings.TrimRight(opts.AssetPath, "/")))
	attrs.WriteString(`/gohtmxelm-interactions.js"`)
	if opts.Debug {
		attrs.WriteString(` data-debug="true"`)
	}
	return "<script" + attrs.String() + "></script>"
}

// InteractionOpenAttrs serialises the common attributes for a button or link
// that opens a server-rendered interaction fragment.
func InteractionOpenAttrs(url string, statusTarget string) string {
	attrs := [][2]string{{"data-gohtmxelm-open", url}}
	if statusTarget != "" {
		attrs = append(attrs, [2]string{"data-gohtmxelm-status", statusTarget})
	}
	return attrsString(attrs)
}

// InteractionResultAttrs serialises the attributes for an element that closes
// the nearest interaction fragment with a result value.
func InteractionResultAttrs(result string) string {
	return attrsString([][2]string{{"data-gohtmxelm-result", result}})
}

func attrsString(attrs [][2]string) string {
	var b strings.Builder
	for _, attr := range attrs {
		b.WriteByte(' ')
		b.WriteString(attr[0])
		b.WriteString(`="`)
		b.WriteString(htmlpkg.EscapeString(attr[1]))
		b.WriteByte('"')
	}
	return b.String()
}

// InteractionEvent is the DOM CustomEvent detail emitted as
// gohtmxelm:interaction-result when an interaction closes.
type InteractionEvent struct {
	Target string `json:"target"`
	Result string `json:"result"`
}

// MarshalInteractionEvent is a small testable helper for hosts that want to
// emit the same event shape from custom JavaScript or SSE payloads.
func MarshalInteractionEvent(target, result string) (string, error) {
	b, err := json.Marshal(InteractionEvent{Target: target, Result: result})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

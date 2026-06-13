package gohtmxelm

import (
	"encoding/json"
	htmlpkg "html"
	"strings"
)

// BrowserScript returns a <script> tag that loads the embedded broker runtime
// and configures it from the supplied Options. SSE sources are serialised into
// a single data-sources attribute the broker reads on boot.
func BrowserScript(opts Options) string {
	if opts.AssetPath == "" {
		opts.AssetPath = "/gohtmxelm"
	}

	var attrs strings.Builder
	attrs.WriteString(` defer src="`)
	attrs.WriteString(htmlpkg.EscapeString(strings.TrimRight(opts.AssetPath, "/")))
	attrs.WriteString(`/gohtmxelm-broker.js"`)
	if len(opts.Sources) > 0 {
		// Marshalling can only fail on unsupported types; Source is plain
		// strings, so the error is unreachable in practice.
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

// ElmIsland returns the HTML mount point convention used by the broker. The
// returned string is fully escaped; in templ wrap it with templ.Raw, and with
// html/template use template.HTML.
func ElmIsland(id string, module string, props any) (string, error) {
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", err
	}
	return `<div class="elm-island" id="` + htmlpkg.EscapeString(id) +
		`" data-elm-module="` + htmlpkg.EscapeString(module) +
		`" data-island-id="` + htmlpkg.EscapeString(id) +
		`" data-props="` + htmlpkg.EscapeString(string(propsJSON)) + `"></div>`, nil
}

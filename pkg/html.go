package gohtmxelm

import (
	"encoding/json"
	htmlpkg "html"
	"strings"
)

// BrowserScript returns a script tag for the embedded browser broker.
func BrowserScript(opts Options) string {
	if opts.AssetPath == "" {
		opts.AssetPath = "/gohtmxelm"
	}

	var attrs strings.Builder
	attrs.WriteString(` defer src="`)
	attrs.WriteString(htmlpkg.EscapeString(strings.TrimRight(opts.AssetPath, "/")))
	attrs.WriteString(`/gohtmxelm-broker.js"`)
	if opts.EventStream != "" {
		attrs.WriteString(` data-events="`)
		attrs.WriteString(htmlpkg.EscapeString(opts.EventStream))
		attrs.WriteString(`"`)
	}
	if len(opts.EventNames) > 0 {
		attrs.WriteString(` data-event-names="`)
		attrs.WriteString(htmlpkg.EscapeString(strings.Join(opts.EventNames, ",")))
		attrs.WriteString(`"`)
	}
	if opts.Debug {
		attrs.WriteString(` data-debug="true"`)
	}
	return "<script" + attrs.String() + "></script>"
}

// ElmIsland returns the HTML mount point convention used by the broker.
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

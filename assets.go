package gohtmxelm

import (
	"embed"
	"io/fs"
	"net/http"
)

// runtimeFS contains the browser runtime that mounts Elm islands and bridges
// Elm ports, HTMX swaps, and optional SSE streams.
//
//go:embed runtime/*
var runtimeFS embed.FS

// Assets returns an HTTP handler for the embedded gohtmxelm browser runtime.
// Mount it under a stable prefix in the host application, for example:
//
//	mux.Handle("/gohtmxelm/", http.StripPrefix("/gohtmxelm/", gohtmxelm.Assets()))
func Assets() http.Handler {
	sub, err := fs.Sub(runtimeFS, "runtime")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}

// Kit bundles reusable gohtmxelm runtime options.
type Kit struct {
	opts Options
}

// Source describes one SSE endpoint the browser broker should connect to and
// the named events it should forward to islands as TypeSSEEvent.
type Source struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

// Options configures the reusable browser integration layer.
type Options struct {
	// AssetPath is the URL path where Assets is mounted. It is used by
	// BrowserScript. Defaults to "/gohtmxelm".
	AssetPath string
	// Sources are the SSE endpoints the broker opens. Each forwarded event is
	// broadcast to islands as a TypeSSEEvent envelope carrying {event, data}.
	// Multiple sources are supported, so independent streams (for example a
	// store stream and a timer stream) can coexist.
	Sources []Source
	// Debug enables browser-side runtime logging.
	Debug bool
}

// New creates a configured reusable integration kit.
func New(opts Options) *Kit {
	if opts.AssetPath == "" {
		opts.AssetPath = "/gohtmxelm"
	}
	return &Kit{opts: opts}
}

// Assets returns the embedded runtime assets for this kit.
func (k *Kit) Assets() http.Handler {
	return Assets()
}

// BrowserScript renders the script tag needed by pages that use Elm islands.
func (k *Kit) BrowserScript() string {
	return BrowserScript(k.opts)
}

// InteractionScript renders the script tag needed by pages that use the
// server-rendered interaction overlay convention.
func (k *Kit) InteractionScript() string {
	return InteractionScript(k.opts)
}

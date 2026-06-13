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

// Options configures the reusable browser integration layer.
type Options struct {
	// AssetPath is the URL path where Assets is mounted. It is used by
	// BrowserScript. Defaults to "/gohtmxelm".
	AssetPath string
	// EventStream is an optional SSE endpoint consumed by the generic broker.
	// Named events listed in EventNames are forwarded to Elm as SSE_EVENT.
	EventStream string
	// EventNames are the named SSE events the browser broker should listen for.
	EventNames []string
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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/nkhine/gohtmxelm/demo/internal/stopwatch"
	"github.com/nkhine/gohtmxelm/demo/internal/store"
	"github.com/nkhine/gohtmxelm/demo/internal/ui"
	"github.com/nkhine/gohtmxelm/demo/internal/ui/components"
	gohtmxelm "github.com/nkhine/gohtmxelm/pkg"
)

type exampleRoute struct {
	Slug        string
	Title       string
	Description string
	Render      func(stopwatch.Snapshot) templ.Component
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	kv := store.New()
	kv.Set("greeting", "hello world")
	kv.Set("status", "running")

	// Shared lifecycle context: cancels on SIGINT/SIGTERM and stops the
	// stopwatch tick goroutine cleanly alongside the HTTP server.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sw := stopwatch.New()
	go sw.Run(ctx)

	exampleRoutes := []exampleRoute{
		{
			Slug:        "message",
			Title:       "Shared message workbench",
			Description: "HTMX, Datastar, Elm, and Go update one shared key.",
			Render: func(stopwatch.Snapshot) templ.Component {
				return components.MessageWorkbench()
			},
		},
		{
			Slug:        "stopwatch",
			Title:       "Hello stopwatch",
			Description: "HTMX controls a Go timer while Datastar and Elm react to SSE.",
			Render: func(snap stopwatch.Snapshot) templ.Component {
				return components.StopwatchExample(snap.Running, snap.CanLap())
			},
		},
	}
	exampleNav := navItems(exampleRoutes)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("demo/static"))))
	// The reusable broker runtime is served straight from the package's
	// embedded assets — the demo runs the exact code it ships.
	r.Handle("/gohtmxelm/*", http.StripPrefix("/gohtmxelm/", gohtmxelm.Assets()))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		snap := sw.Snapshot()
		if err := ui.IndexPage(exampleNav, snap.Running, snap.CanLap()).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	r.Get("/examples/{slug}", func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		example, ok := findExample(exampleRoutes, slug)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if err := ui.ExamplePage(exampleNav, slug, example.Title, example.Render(sw.Snapshot())).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	r.Get("/message", func(w http.ResponseWriter, r *http.Request) {
		if err := ui.ServerMessage("Hello from Go via HTMX").Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Store fragment rendered by HTMX on load and on every store-refresh trigger.
	r.Get("/api/store/fragment", func(w http.ResponseWriter, r *http.Request) {
		if err := ui.StoreEntries(kv.Entries()).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Datastar owns its own DOM island. The browser opens this stream with
	// data-init="@get(...)" and applies datastar-patch-elements events directly.
	r.Get("/api/datastar/store/stream", func(w http.ResponseWriter, r *http.Request) {
		stream, err := gohtmxelm.NewStream(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = gohtmxelm.Serve(stream, kv.Events(),
			func(s *gohtmxelm.Stream) error {
				if err := s.PatchElements(render(components.DatastarStore(kv.Entries(), "Initial Datastar snapshot from Go"))); err != nil {
					return err
				}
				return s.PatchSignals(map[string]any{"writes": kv.Seq(), "lastWriter": ""})
			},
			func(s *gohtmxelm.Stream, e store.Event) error {
				note := fmt.Sprintf("Go store changed: %s (by %s)", e.Key, e.Source)
				if e.Deleted {
					note = fmt.Sprintf("Go store deleted: %s (by %s)", e.Key, e.Source)
				}
				if err := s.PatchElements(render(components.DatastarStore(kv.Entries(), note))); err != nil {
					return err
				}
				// Go also pushes signal patches: the live counters update with
				// zero client-side wiring.
				return s.PatchSignals(map[string]any{"writes": e.Seq, "lastWriter": e.Source})
			},
		)
	})

	// Datastar stopwatch stream: hydrate the whole region on connect, then push
	// the readout every tick but the lap list only when it can have changed.
	r.Get("/api/stopwatch/stream", func(w http.ResponseWriter, r *http.Request) {
		stream, err := gohtmxelm.NewStream(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = gohtmxelm.Serve(stream, sw.Events(),
			func(s *gohtmxelm.Stream) error {
				snap := sw.Snapshot()
				if err := s.PatchElements(render(components.StopwatchReadout(snap))); err != nil {
					return err
				}
				return s.PatchElements(render(components.StopwatchLaps(snap)))
			},
			func(s *gohtmxelm.Stream, ev stopwatch.Event) error {
				if err := s.PatchElements(render(components.StopwatchReadout(ev.Snapshot))); err != nil {
					return err
				}
				if ev.StateChange {
					return s.PatchElements(render(components.StopwatchLaps(ev.Snapshot)))
				}
				return nil
			},
		)
	})

	// JSON stopwatch stream consumed by the broker. Emits only on discrete
	// state changes (start/stop/lap/reset), not on ticks: demo-ui.js forwards
	// these to the Elm lap analyzer and triggers an HTMX controls refresh so
	// every connected tab converges on the same control state.
	r.Get("/api/stopwatch/events", func(w http.ResponseWriter, r *http.Request) {
		stream, err := gohtmxelm.NewStream(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = gohtmxelm.Serve(stream, sw.Events(),
			func(s *gohtmxelm.Stream) error { return s.Send("stopwatch-state", sw.Snapshot()) },
			func(s *gohtmxelm.Stream, ev stopwatch.Event) error {
				if !ev.StateChange {
					return nil
				}
				return s.Send("stopwatch-state", ev.Snapshot)
			},
		)
	})

	// Controls fragment GET, used both on initial load and to re-render controls
	// in tabs that did not initiate the change (HTMX hx-trigger fired by
	// demo-ui.js on the SSE state event).
	r.Get("/api/stopwatch/controls", func(w http.ResponseWriter, r *http.Request) {
		renderControls(w, r, sw.Snapshot())
	})
	r.Post("/api/stopwatch/start", func(w http.ResponseWriter, r *http.Request) {
		renderControls(w, r, sw.Start())
	})
	r.Post("/api/stopwatch/stop", func(w http.ResponseWriter, r *http.Request) {
		renderControls(w, r, sw.Stop())
	})
	r.Post("/api/stopwatch/reset", func(w http.ResponseWriter, r *http.Request) {
		renderControls(w, r, sw.Reset())
	})
	r.Post("/api/stopwatch/lap", func(w http.ResponseWriter, r *http.Request) {
		renderControls(w, r, sw.Lap())
	})

	// Set a key/value.
	// Accepts application/x-www-form-urlencoded (HTMX form, no version check) or
	// application/json (broker, optional version for optimistic locking).
	// Returns 409 Conflict when a versioned write fails.
	r.Post("/api/store", func(w http.ResponseWriter, r *http.Request) {
		key, value, source, version, err := parseStoreBody(r)
		if err != nil || key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}
		if _, ok := kv.SetIf(key, value, sanitizeSource(source), version); !ok {
			http.Error(w, "version conflict", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Delete a key. The HTMX store table renders one hx-delete control per row.
	r.Delete("/api/store/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		if !kv.Delete(key, "htmx") {
			http.Error(w, "no such key", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Datastar form writes return SSE events, because Datastar's fetch action
	// expects the response to be hypermedia it can apply immediately.
	r.Post("/api/datastar/store", func(w http.ResponseWriter, r *http.Request) {
		stream, err := gohtmxelm.NewStream(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		key, value, _, version, err := parseStoreBody(r)
		if err != nil || strings.TrimSpace(key) == "" {
			_ = stream.PatchElements(render(components.DatastarWriteResult("Key is required.", true)))
			return
		}
		if _, ok := kv.SetIf(key, value, "datastar", version); !ok {
			_ = stream.PatchElements(render(components.DatastarWriteResult("Version conflict; the live stream will show the winning value.", true)))
			return
		}
		_ = stream.PatchElements(render(components.DatastarWriteResult(fmt.Sprintf("Saved %q via Datastar.", key), false)))
		_ = stream.PatchSignals(map[string]any{"messageDraft": ""})
	})

	// SSE stream consumed by the broker.
	// On connect: emits one store-hydrate event per key so demo-ui.js can
	// initialise per-key versions before processing deltas.
	// Ongoing: emits store-change events carrying seq and version so clients
	// can detect and discard out-of-order / stale deliveries.
	r.Get("/api/events", func(w http.ResponseWriter, r *http.Request) {
		stream, err := gohtmxelm.NewStream(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = gohtmxelm.Serve(stream, kv.Events(),
			func(s *gohtmxelm.Stream) error {
				for _, state := range kv.AllStates() {
					if err := s.Send("store-hydrate", state); err != nil {
						return err
					}
				}
				return nil
			},
			func(s *gohtmxelm.Stream, e store.Event) error {
				return s.Send("store-change", e)
			},
		)
	})

	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0, // disabled — SSE connections are long-lived
		IdleTimeout:  120 * time.Second,
		// Derive every request context from the signal context so that on
		// SIGINT/SIGTERM all in-flight SSE handlers unblock via r.Context()
		// and return normally, letting net/http finish each chunked stream
		// cleanly instead of being hard-killed mid-stream.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	go func() {
		logger.Info("server started", "addr", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}

// renderControls renders the stopwatch control buttons for the given snapshot.
func renderControls(w http.ResponseWriter, r *http.Request, snap stopwatch.Snapshot) {
	if err := components.StopwatchControls(snap.Running, snap.CanLap()).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// render renders a templ component to a string for SSE patch bodies, keeping a
// single rendering path (templ) for both full pages and streamed fragments.
func render(c templ.Component) string {
	var b strings.Builder
	_ = c.Render(context.Background(), &b)
	return b.String()
}

// parseStoreBody reads key, value, source, and optional version from either a
// JSON body (broker) or form data (HTMX). Form submissions always use
// version=0 (no optimistic locking) and default to source "htmx".
func parseStoreBody(r *http.Request) (key, value, source string, version uint64, err error) {
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Key     string `json:"key"`
			Value   string `json:"value"`
			Source  string `json:"source"`
			Version uint64 `json:"version"`
		}
		err = json.NewDecoder(r.Body).Decode(&body)
		return body.Key, body.Value, body.Source, body.Version, err
	}
	source = r.FormValue("source")
	if source == "" {
		source = "htmx"
	}
	return r.FormValue("key"), r.FormValue("value"), source, 0, nil
}

// sanitizeSource constrains client-supplied attribution to a short lowercase
// slug so it is safe to echo into every rendering surface.
func sanitizeSource(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
		if b.Len() >= 24 {
			break
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func navItems(routes []exampleRoute) []ui.ExampleNav {
	items := make([]ui.ExampleNav, 0, len(routes))
	for _, route := range routes {
		items = append(items, ui.ExampleNav{
			Slug:        route.Slug,
			Title:       route.Title,
			Description: route.Description,
		})
	}
	return items
}

func findExample(routes []exampleRoute, slug string) (exampleRoute, bool) {
	for _, route := range routes {
		if route.Slug == slug {
			return route, true
		}
	}
	return exampleRoute{}, false
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

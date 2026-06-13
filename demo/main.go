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

	"github.com/nkhine/gohtmxelm/demo/internal/statement"
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

	stmt := statement.New(time.Now)

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
		{
			Slug:        "statement",
			Title:       "Account statement",
			Description: "Elm range picker filters Go-owned transfers; HTMX + Datastar render.",
			Render: func(stopwatch.Snapshot) templ.Component {
				return components.StatementExample()
			},
		},
	}
	exampleNav := navItems(exampleRoutes)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
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

	// Quiet the browser's automatic favicon request so it doesn't surface as a
	// 404 in the console.
	r.Get("/favicon.ico", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

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

	// Single multiplexed broker stream. The browser broker holds one EventSource
	// open per source; carrying every domain's events on one connection keeps
	// the page well under the ~6-connection HTTP/1.1 limit even when several
	// examples (each with its own Datastar stream) share a page.
	//
	// On connect it hydrates every domain (store keys, stopwatch state, the
	// active statement range); thereafter it forwards each domain's changes.
	// Stopwatch ticks are skipped — only discrete state changes are relevant to
	// the broker and the Elm lap analyzer.
	r.Get("/api/stream", func(w http.ResponseWriter, r *http.Request) {
		stream, err := gohtmxelm.NewStream(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		storeCh := kv.Events().Subscribe()
		defer kv.Events().Unsubscribe(storeCh)
		swCh := sw.Events().Subscribe()
		defer sw.Events().Unsubscribe(swCh)
		stCh := stmt.Events().Subscribe()
		defer stmt.Events().Unsubscribe(stCh)

		// Hydrate every domain on connect.
		for _, state := range kv.AllStates() {
			if stream.Send("store-hydrate", state) != nil {
				return
			}
		}
		_ = stream.Send("stopwatch-state", sw.Snapshot())
		rng := stmt.Range()
		_ = stream.Send("statement-range-change", rangePayload(rng, stmt.Summary(rng)))

		for {
			select {
			case e, ok := <-storeCh:
				if !ok {
					return
				}
				if stream.Send("store-change", e) != nil {
					return
				}
			case ev, ok := <-swCh:
				if !ok {
					return
				}
				if ev.StateChange && stream.Send("stopwatch-state", ev.Snapshot) != nil {
					return
				}
			case ev, ok := <-stCh:
				if !ok {
					return
				}
				if stream.Send("statement-range-change", rangePayload(ev.Range, ev.Summary)) != nil {
					return
				}
			case <-stream.Done():
				return
			}
		}
	})

	// ── Bank statement example ────────────────────────────────────────────
	// HTMX renders the statement table for the current server-selected range.
	r.Get("/api/statement/transfers", func(w http.ResponseWriter, r *http.Request) {
		rng := stmt.Range()
		opening := stmt.Summary(rng).OpeningMinor
		if err := components.StatementTransfers(stmt.Transfers(rng), opening).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// The Elm picker's selection arrives here (mirrored by demo-ui.js). Go
	// resolves a preset against the server clock or parses a custom window,
	// sets the active range, and fans the change out over SSE.
	r.Post("/api/statement/range", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RelValue int    `json:"relValue"`
			RelUnit  string `json:"relUnit"`
			From     string `json:"from"`
			To       string `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		var err error
		if body.RelUnit != "" {
			_, err = stmt.ApplyRelative(body.RelValue, body.RelUnit)
		} else {
			_, err = stmt.ApplyCustom(body.From, body.To)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Datastar owns the summary panel: signal patches drive the headline
	// counters and an element patch re-renders the detailed grid on every
	// range change.
	r.Get("/api/statement/stream", func(w http.ResponseWriter, r *http.Request) {
		stream, err := gohtmxelm.NewStream(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		patch := func(s *gohtmxelm.Stream, rng statement.Range, sum statement.Summary) error {
			if err := s.PatchElements(render(components.StatementSummary(sum))); err != nil {
				return err
			}
			return s.PatchSignals(statementSignals(rng, sum))
		}
		_ = gohtmxelm.Serve(stream, stmt.Events(),
			func(s *gohtmxelm.Stream) error {
				rng := stmt.Range()
				return patch(s, rng, stmt.Summary(rng))
			},
			func(s *gohtmxelm.Stream, ev statement.RangeEvent) error {
				return patch(s, ev.Range, ev.Summary)
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

// statementSignals are the Datastar signals patched on every range change —
// the headline counters (and the selected period) that update with no
// client-side wiring.
func statementSignals(rng statement.Range, sum statement.Summary) map[string]any {
	return map[string]any{
		"period":  rng.Label,
		"count":   sum.Count,
		"opening": statement.FormatGBP(sum.OpeningMinor),
		"credits": statement.FormatGBP(sum.CreditsMinor),
		"debits":  statement.FormatGBP(sum.DebitsMinor),
		"closing": statement.FormatGBP(sum.ClosingMinor),
	}
}

// rangePayload is the JSON the broker forwards to the Elm picker and demo-ui.js.
// todayIso lets the calendar open on the current month without the island
// needing its own clock or a timezone library.
func rangePayload(rng statement.Range, sum statement.Summary) map[string]any {
	return map[string]any{
		"label":    rng.Label,
		"count":    sum.Count,
		"fromMs":   rng.From.UnixMilli(),
		"toMs":     rng.To.UnixMilli(),
		"todayIso": time.Now().Format("2006-01-02"),
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

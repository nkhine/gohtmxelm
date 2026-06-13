package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	htmlpkg "html"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"elm-htmx-templ-demo/internal/store"
	"elm-htmx-templ-demo/templates"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	kv := store.New()
	kv.Set("greeting", "hello world")
	kv.Set("status", "running")

	// Shared lifecycle context: cancels on SIGINT/SIGTERM and stops the
	// stopwatch tick goroutine cleanly alongside the HTTP server.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	stopwatch := NewStopwatch()
	go stopwatch.Run(ctx)

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

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		snap := stopwatch.Snapshot()
		if err := templates.Page(snap.Running, stopwatchCanLap(snap)).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	r.Get("/message", func(w http.ResponseWriter, r *http.Request) {
		if err := templates.ServerMessage("Hello from Go via HTMX").Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Store fragment rendered by HTMX on load and on every store-refresh trigger.
	r.Get("/api/store/fragment", func(w http.ResponseWriter, r *http.Request) {
		if err := templates.StoreEntries(kv.Entries()).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Datastar owns its own DOM island. The browser opens this stream with
	// data-init="@get(...)" and applies datastar-patch-elements events directly.
	r.Get("/api/datastar/store/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := kv.Subscribe()
		defer kv.Unsubscribe(ch)

		writeDatastarPatchElements(w, renderDatastarStore(kv.Entries(), "Initial Datastar snapshot from Go"))
		writeDatastarPatchSignals(w, fmt.Sprintf(`{"writes": %d, "lastWriter": ""}`, kv.Seq()))
		flusher.Flush()

		for {
			select {
			case e, ok := <-ch:
				if !ok {
					return
				}
				note := fmt.Sprintf("Go store changed: %s (by %s)", e.Key, e.Source)
				if e.Deleted {
					note = fmt.Sprintf("Go store deleted: %s (by %s)", e.Key, e.Source)
				}
				writeDatastarPatchElements(w, renderDatastarStore(kv.Entries(), note))
				// Go also pushes signal patches: the live counters in the
				// Datastar panel update with zero client-side wiring.
				writeDatastarPatchSignals(w, fmt.Sprintf(`{"writes": %d, "lastWriter": %q}`, e.Seq, e.Source))
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	r.Get("/api/stopwatch/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := stopwatch.Subscribe()
		defer stopwatch.Unsubscribe(ch)

		// Hydrate the whole region on connect; afterwards push the readout
		// every tick but the lap list only when it can have changed (state
		// changes), so the 10/sec tick stream stays small.
		snap := stopwatch.Snapshot()
		writeDatastarPatchElements(w, renderStopwatchReadout(snap))
		writeDatastarPatchElements(w, renderStopwatchLaps(snap))
		flusher.Flush()

		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				writeDatastarPatchElements(w, renderStopwatchReadout(ev.Snapshot))
				if ev.StateChange {
					writeDatastarPatchElements(w, renderStopwatchLaps(ev.Snapshot))
				}
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	// JSON stopwatch stream consumed by broker.js. Emits only on discrete
	// state changes (start/stop/lap/reset), not on ticks: broker.js forwards
	// these to the Elm lap analyzer and triggers an HTMX controls refresh so
	// every connected tab converges on the same control state.
	r.Get("/api/stopwatch/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := stopwatch.Subscribe()
		defer stopwatch.Unsubscribe(ch)

		writeSSE(w, "stopwatch-state", stopwatch.Snapshot())
		flusher.Flush()

		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				if !ev.StateChange {
					continue
				}
				writeSSE(w, "stopwatch-state", ev.Snapshot)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	// Controls fragment GET, used both on initial load and to re-render
	// controls in tabs that did not initiate the change (HTMX hx-trigger
	// fired by broker.js on the SSE state event).
	r.Get("/api/stopwatch/controls", func(w http.ResponseWriter, r *http.Request) {
		snap := stopwatch.Snapshot()
		if err := templates.StopwatchControls(snap.Running, stopwatchCanLap(snap)).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	r.Post("/api/stopwatch/start", func(w http.ResponseWriter, r *http.Request) {
		snap := stopwatch.Start()
		if err := templates.StopwatchControls(snap.Running, stopwatchCanLap(snap)).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	r.Post("/api/stopwatch/stop", func(w http.ResponseWriter, r *http.Request) {
		snap := stopwatch.Stop()
		if err := templates.StopwatchControls(snap.Running, stopwatchCanLap(snap)).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	r.Post("/api/stopwatch/reset", func(w http.ResponseWriter, r *http.Request) {
		snap := stopwatch.Reset()
		if err := templates.StopwatchControls(snap.Running, stopwatchCanLap(snap)).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	r.Post("/api/stopwatch/lap", func(w http.ResponseWriter, r *http.Request) {
		snap := stopwatch.Lap()
		if err := templates.StopwatchControls(snap.Running, stopwatchCanLap(snap)).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Set a key/value.
	// Accepts application/x-www-form-urlencoded (HTMX form, no version check) or
	// application/json (broker.js, optional version for optimistic locking).
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

	// Delete a key. The HTMX store table renders one hx-delete control per
	// row — hypermedia as the engine of application state, then SSE fan-out.
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
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")

		key, value, _, version, err := parseStoreBody(r)
		if err != nil || strings.TrimSpace(key) == "" {
			writeDatastarPatchElements(w, datastarWriteResult("Key is required.", true))
			return
		}
		if _, ok := kv.SetIf(key, value, "datastar", version); !ok {
			writeDatastarPatchElements(w, datastarWriteResult("Version conflict; the live stream will show the winning value.", true))
			return
		}

		writeDatastarPatchElements(w, datastarWriteResult(fmt.Sprintf("Saved %q via Datastar.", key), false))
		writeDatastarPatchSignals(w, `{"messageDraft":""}`)
	})

	// SSE stream.
	// On connect: emits one store-hydrate event per key so the client can
	// initialise storeVersions before processing deltas.
	// Ongoing: emits store-change events carrying seq and version so clients
	// can detect and discard out-of-order / stale deliveries.
	// WriteTimeout is disabled on the server for long-lived SSE connections;
	// context cancellation handles client disconnects.
	r.Get("/api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := kv.Subscribe()
		defer kv.Unsubscribe(ch)

		// Hydrate client with full current state under a distinct event name so
		// broker.js applies it unconditionally (regardless of storeSeq).
		for _, state := range kv.AllStates() {
			writeSSE(w, "store-hydrate", state)
		}
		flusher.Flush()

		for {
			select {
			case e, ok := <-ch:
				if !ok {
					return
				}
				writeSSE(w, "store-change", e)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0, // disabled — SSE connections are long-lived
		IdleTimeout:  120 * time.Second,
		// Derive every request context from the signal context so that on
		// SIGINT/SIGTERM all in-flight SSE handlers unblock via r.Context()
		// and return normally. That lets net/http finish each chunked stream
		// cleanly (so the browser sees a complete response and EventSource
		// simply reconnects) instead of the process being hard-killed
		// mid-stream, which surfaces as ERR_INCOMPLETE_CHUNKED_ENCODING.
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

	// SSE handlers return promptly now that their request contexts are
	// cancelled, so a short grace period is enough.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}

// parseStoreBody reads key, value, source, and optional version from either a
// JSON body (broker.js) or form data (HTMX). Form submissions always use
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

// sanitizeSource constrains client-supplied attribution to a short
// lowercase slug so it is safe to echo into every rendering surface.
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

func writeSSE(w http.ResponseWriter, event string, data any) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
}

func writeDatastarPatchElements(w http.ResponseWriter, elements string) {
	fmt.Fprint(w, "event: datastar-patch-elements\n")
	for _, line := range strings.Split(elements, "\n") {
		fmt.Fprintf(w, "data: elements %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func writeDatastarPatchSignals(w http.ResponseWriter, signals string) {
	fmt.Fprint(w, "event: datastar-patch-signals\n")
	fmt.Fprintf(w, "data: signals %s\n\n", signals)
}

func renderDatastarStore(entries []store.Entry, note string) string {
	var b strings.Builder
	b.WriteString(`<div id="datastar-store" class="datastar-store">`)
	fmt.Fprintf(&b, `<p class="datastar-note">%s</p>`, htmlpkg.EscapeString(note))
	if len(entries) == 0 {
		b.WriteString(`<p class="muted">Store is empty.</p>`)
	} else {
		b.WriteString(`<table><thead><tr><th>Key</th><th>Value</th><th>By</th></tr></thead><tbody>`)
		for _, e := range entries {
			fmt.Fprintf(
				&b,
				`<tr data-datastar-key="%s"><td>%s</td><td>%s</td><td><span class="source-chip source-%s">%s</span></td></tr>`,
				htmlpkg.EscapeString(e.Key),
				htmlpkg.EscapeString(e.Key),
				htmlpkg.EscapeString(e.Value),
				htmlpkg.EscapeString(e.Source),
				htmlpkg.EscapeString(e.Source),
			)
		}
		b.WriteString(`</tbody></table>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func datastarWriteResult(message string, isError bool) string {
	className := "datastar-result"
	if isError {
		className += " error"
	}
	return fmt.Sprintf(
		`<div id="datastar-write-result" class="%s">%s</div>`,
		className,
		htmlpkg.EscapeString(message),
	)
}

type StopwatchSnapshot struct {
	ElapsedMs int64          `json:"elapsedMs"`
	Running   bool           `json:"running"`
	Laps      []StopwatchLap `json:"laps"`
}

type StopwatchLap struct {
	Number    int   `json:"number"`
	ElapsedMs int64 `json:"elapsedMs"`
}

// StopwatchEvent is what subscribers receive. StateChange distinguishes a
// discrete user action (start/stop/lap/reset) from a periodic tick, so
// consumers that only care about control state (HTMX controls, the Elm lap
// analyzer) can ignore the 10/sec tick stream.
type StopwatchEvent struct {
	Snapshot    StopwatchSnapshot
	StateChange bool
}

type Stopwatch struct {
	mu          sync.RWMutex
	elapsed     time.Duration
	startedAt   time.Time
	running     bool
	laps        []time.Duration
	subscribers map[chan StopwatchEvent]struct{}
	now         func() time.Time // injectable clock for deterministic tests
	wake        chan struct{}    // signals Run that the timer started ticking
}

func NewStopwatch() *Stopwatch {
	return &Stopwatch{
		subscribers: make(map[chan StopwatchEvent]struct{}),
		now:         time.Now,
		wake:        make(chan struct{}, 1),
	}
}

// Run drives the periodic tick loop. The ticker only runs while the stopwatch
// is running: when paused it stops the ticker and blocks on wake, so an idle
// stopwatch does no work. Start() pokes wake to resume ticking.
func (s *Stopwatch) Run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	ticker.Stop()
	defer ticker.Stop()

	running := false
	for {
		if running {
			select {
			case <-ticker.C:
				if _, ok := s.tick(); !ok {
					ticker.Stop()
					running = false
				}
			case <-s.wake:
				// already ticking; nothing to do
			case <-ctx.Done():
				return
			}
		} else {
			select {
			case <-s.wake:
				ticker.Reset(100 * time.Millisecond)
				running = true
			case <-ctx.Done():
				return
			}
		}
	}
}

func (s *Stopwatch) Start() StopwatchSnapshot {
	s.mu.Lock()
	if !s.running {
		s.running = true
		s.startedAt = s.now()
	}
	snap := s.snapshotLocked()
	subs := s.subscriberListLocked()
	s.mu.Unlock()
	s.notify(subs, StopwatchEvent{Snapshot: snap, StateChange: true})
	// Wake the tick loop without blocking if a poke is already pending.
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return snap
}

func (s *Stopwatch) Stop() StopwatchSnapshot {
	s.mu.Lock()
	if s.running {
		s.elapsed += s.now().Sub(s.startedAt)
		s.running = false
	}
	snap := s.snapshotLocked()
	subs := s.subscriberListLocked()
	s.mu.Unlock()
	s.notify(subs, StopwatchEvent{Snapshot: snap, StateChange: true})
	return snap
}

func (s *Stopwatch) Reset() StopwatchSnapshot {
	s.mu.Lock()
	s.elapsed = 0
	s.startedAt = s.now()
	s.running = false
	s.laps = nil
	snap := s.snapshotLocked()
	subs := s.subscriberListLocked()
	s.mu.Unlock()
	s.notify(subs, StopwatchEvent{Snapshot: snap, StateChange: true})
	return snap
}

func (s *Stopwatch) Lap() StopwatchSnapshot {
	s.mu.Lock()
	elapsed := s.elapsedLocked()
	if elapsed > 0 {
		s.laps = append([]time.Duration{elapsed}, s.laps...)
	}
	snap := s.snapshotLocked()
	subs := s.subscriberListLocked()
	s.mu.Unlock()
	s.notify(subs, StopwatchEvent{Snapshot: snap, StateChange: true})
	return snap
}

func (s *Stopwatch) Snapshot() StopwatchSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *Stopwatch) Subscribe() chan StopwatchEvent {
	ch := make(chan StopwatchEvent, 16)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *Stopwatch) Unsubscribe(ch chan StopwatchEvent) {
	s.mu.Lock()
	delete(s.subscribers, ch)
	s.mu.Unlock()
	close(ch)
}

// tick emits a periodic (non-state-change) update. It returns ok=false when
// the stopwatch is paused, which tells Run to stop the ticker.
func (s *Stopwatch) tick() (StopwatchSnapshot, bool) {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return StopwatchSnapshot{}, false
	}
	snap := s.snapshotLocked()
	subs := s.subscriberListLocked()
	s.mu.Unlock()
	s.notify(subs, StopwatchEvent{Snapshot: snap, StateChange: false})
	return snap, true
}

func (s *Stopwatch) snapshotLocked() StopwatchSnapshot {
	elapsed := s.elapsedLocked()
	laps := make([]StopwatchLap, 0, len(s.laps))
	total := len(s.laps)
	for i, lap := range s.laps {
		laps = append(laps, StopwatchLap{
			Number:    total - i,
			ElapsedMs: lap.Milliseconds(),
		})
	}
	return StopwatchSnapshot{
		ElapsedMs: elapsed.Milliseconds(),
		Running:   s.running,
		Laps:      laps,
	}
}

func (s *Stopwatch) elapsedLocked() time.Duration {
	elapsed := s.elapsed
	if s.running {
		elapsed += s.now().Sub(s.startedAt)
	}
	return elapsed
}

func (s *Stopwatch) subscriberListLocked() []chan StopwatchEvent {
	subs := make([]chan StopwatchEvent, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subs = append(subs, ch)
	}
	return subs
}

func (s *Stopwatch) notify(subs []chan StopwatchEvent, ev StopwatchEvent) {
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// renderStopwatchReadout renders only the clock face — pushed on every tick.
// The face is four concentric rings (hours, minutes, seconds, subseconds),
// each filled by how far through its own unit the elapsed time currently is.
func renderStopwatchReadout(snap StopwatchSnapshot) string {
	status := "paused"
	if snap.Running {
		status = "running"
	}
	sub, sec, min, hour := stopwatchDials(snap.ElapsedMs)

	var b strings.Builder
	b.WriteString(`<div id="stopwatch-readout">`)
	fmt.Fprintf(
		&b,
		`<div class="dial %s" style="--d-sub:%.2fdeg;--d-sec:%.2fdeg;--d-min:%.2fdeg;--d-hour:%.2fdeg">`,
		status, sub, sec, min, hour,
	)
	// The dial-center is nested inside the innermost ring so each ring shows
	// only as a band around its child.
	b.WriteString(`<div class="ring ring-hour"><div class="ring ring-min"><div class="ring ring-sec"><div class="ring ring-sub">`)
	b.WriteString(`<div class="dial-center">`)
	fmt.Fprintf(&b, `<div class="stopwatch-time">%s</div>`, htmlpkg.EscapeString(formatElapsed(snap.ElapsedMs)))
	fmt.Fprintf(&b, `<div class="stopwatch-state">%s</div>`, htmlpkg.EscapeString(status))
	b.WriteString(`</div>`)                  // .dial-center
	b.WriteString(`</div></div></div></div>`) // ring-sub, ring-sec, ring-min, ring-hour
	b.WriteString(`</div>`)                   // .dial
	b.WriteString(`</div>`)                   // #stopwatch-readout
	return b.String()
}

// renderStopwatchLaps renders the lap list and live status — pushed only on
// discrete state changes, since ticks never alter laps.
func renderStopwatchLaps(snap StopwatchSnapshot) string {
	status := "paused"
	if snap.Running {
		status = "running"
	}

	var b strings.Builder
	b.WriteString(`<div id="stopwatch-laps">`)
	b.WriteString(`<div class="lap-list">`)
	if len(snap.Laps) == 0 {
		b.WriteString(`<p class="muted">No laps yet. Use the HTMX Lap button once the timer has started.</p>`)
	} else {
		for _, lap := range snap.Laps {
			fmt.Fprintf(
				&b,
				`<div class="lap-row"><span class="lap-index">#%d</span><span>%s</span></div>`,
				lap.Number,
				htmlpkg.EscapeString(formatElapsed(lap.ElapsedMs)),
			)
		}
	}
	b.WriteString(`</div>`)
	fmt.Fprintf(&b, `<span class="stopwatch-live-status %s">%s via Datastar SSE</span>`, htmlpkg.EscapeString(status), htmlpkg.EscapeString(status))
	b.WriteString(`</div>`)
	return b.String()
}

// formatElapsed formats milliseconds as HH:MM:SS:mmm.
func formatElapsed(ms int64) string {
	hours := ms / 3600000
	minutes := (ms / 60000) % 60
	seconds := (ms / 1000) % 60
	millis := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d:%03d", hours, minutes, seconds, millis)
}

// stopwatchDials returns the fill angle (degrees, 0–360) for each of the four
// dial rings. Each ring shows how far the elapsed time has progressed through
// its own unit: the subsecond ring completes once per second, the second ring
// once per minute, the minute ring once per hour, and the hour ring once per
// 12-hour cycle. Using the full millisecond value (rather than the integer
// unit) makes each ring sweep smoothly rather than stepping.
func stopwatchDials(ms int64) (sub, sec, min, hour float64) {
	const deg = 360.0
	sub = float64(ms%1000) / 1000.0 * deg          // fills every second
	sec = float64(ms%60000) / 60000.0 * deg        // fills every minute
	min = float64(ms%3600000) / 3600000.0 * deg    // fills every hour
	hour = float64(ms%43200000) / 43200000.0 * deg // fills every 12 hours
	return sub, sec, min, hour
}

func stopwatchCanLap(snap StopwatchSnapshot) bool {
	return snap.Running || snap.ElapsedMs > 0
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

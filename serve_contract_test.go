package gohtmxelm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/nkhine/gohtmxelm/simnet"
)

// snapshot is the event payload used by these tests: the full authoritative
// state plus its version. Sending full snapshots is the semantics that makes a
// lossy SSE stream safe — the property the simnet harness verifies abstractly
// and these tests verify against the real Stream/Serve code.
type snapshot struct {
	Version int               `json:"version"`
	Data    map[string]string `json:"data"`
}

// fakeSSE is an in-bubble ResponseWriter+Flusher so Serve's real SSE path runs
// under synctest without a network. It records everything written.
type fakeSSE struct {
	mu     sync.Mutex
	hdr    http.Header
	buf    strings.Builder
	closed bool
}

func newFakeSSE() *fakeSSE { return &fakeSSE{hdr: make(http.Header)} }

func (f *fakeSSE) Header() http.Header { return f.hdr }
func (f *fakeSSE) WriteHeader(int)     {}
func (f *fakeSSE) Flush()              {}
func (f *fakeSSE) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.WriteString(string(p))
}
func (f *fakeSSE) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.String()
}

// TestServe_HydrateThenStream: Serve must emit the hydrate snapshot exactly
// once up front, then forward each published change — the Subscribe→hydrate→
// fan-out lifecycle in stream.go, driven as real goroutines.
func TestServe_HydrateThenStream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBroadcaster[snapshot](16)
		auth := &snapshot{Version: 0, Data: map[string]string{}}

		w := newFakeSSE()
		ctx, cancel := context.WithCancel(context.Background())
		r := httptest.NewRequest("GET", "/stream", nil).WithContext(ctx)
		s, err := NewStream(w, r)
		if err != nil {
			t.Fatalf("NewStream: %v", err)
		}

		go func() {
			_ = Serve(s, b,
				func(s *Stream) error { return s.Send("hydrate", auth) },
				func(s *Stream, ev snapshot) error { return s.Send("update", ev) },
			)
		}()

		synctest.Wait() // subscriber registered, hydrate written

		// Three authoritative writes, published as full snapshots.
		for i := 1; i <= 3; i++ {
			auth.Version = i
			auth.Data[fmt.Sprintf("k%d", i)] = fmt.Sprintf("v%d", i)
			b.Publish(*auth)
			synctest.Wait()
		}

		cancel()        // client disconnects
		synctest.Wait() // Serve returns

		events := parseSSE(w.String())
		if len(events) != 4 {
			t.Fatalf("got %d events, want 4 (1 hydrate + 3 updates):\n%s", len(events), w.String())
		}
		if events[0].name != "hydrate" {
			t.Errorf("first event = %q, want hydrate", events[0].name)
		}
		for i, ev := range events[1:] {
			if ev.name != "update" {
				t.Errorf("event %d = %q, want update", i+1, ev.name)
			}
		}
	})
}

// TestServe_ConvergesAfterDropViaReHydrate is the real-code mirror of the
// simnet contract test. A surface misses a streamed update (the lossy drop),
// then reconnects; Serve's hydrate restores it. We assert convergence with the
// SAME simnet.CheckConvergence used by the model — one bar for both.
func TestServe_ConvergesAfterDropViaReHydrate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBroadcaster[snapshot](16)
		auth := &snapshot{Version: 0, Data: map[string]string{}}

		// First connection: hydrate, receive one update, then a drop (we stop
		// reading and disconnect before the second update is consumed).
		view := connectAndCollect(t, b, auth)

		auth.Version, auth.Data["a"] = 1, "1"
		b.Publish(*auth)
		synctest.Wait()

		view.disconnect() // surface goes dark
		synctest.Wait()

		// Two writes land while the surface is disconnected — both lost.
		auth.Version, auth.Data["b"] = 2, "2"
		b.Publish(*auth)
		auth.Version, auth.Data["c"] = 3, "3"
		b.Publish(*auth)
		synctest.Wait()

		// Reconnect: a fresh Serve hydrates from current authoritative state.
		final := connectAndCollect(t, b, auth)
		synctest.Wait()
		final.disconnect()
		synctest.Wait()

		// The reconnected surface must now present authoritative state exactly.
		got := final.latest()
		if err := simnet.CheckConvergence(
			simnet.Authoritative{Data: auth.Data, Version: auth.Version},
			[]simnet.View{{Label: "reconnected", Data: got.Data, Version: got.Version}},
		); err != nil {
			t.Fatalf("surface did not converge after re-hydrate: %v", err)
		}
		_ = view
	})
}

// TestServe_EndToEndOverHTTP runs Serve behind a real httptest.Server and reads
// the stream over an actual TCP connection — proving the shipped SSE wire
// format and flushing work outside the synctest fake.
func TestServe_EndToEndOverHTTP(t *testing.T) {
	b := NewBroadcaster[snapshot](16)
	auth := snapshot{Version: 7, Data: map[string]string{"x": "y"}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := NewStream(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = Serve(s, b,
			func(s *Stream) error { return s.Send("hydrate", auth) },
			func(s *Stream, ev snapshot) error { return s.Send("update", ev) },
		)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read the hydrate event off the live socket.
	sc := bufio.NewScanner(resp.Body)
	var name, data string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if name != "" {
				goto done
			}
		}
	}
done:
	if name != "hydrate" {
		t.Fatalf("first event = %q, want hydrate", name)
	}
	var snap snapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		t.Fatalf("decode hydrate: %v (data=%q)", err, data)
	}
	if snap.Version != 7 || snap.Data["x"] != "y" {
		t.Fatalf("hydrate payload = %+v, want version 7 / x=y", snap)
	}
}

// --- test helpers ---------------------------------------------------------

type sseEvent struct {
	name string
	data string
}

func parseSSE(raw string) []sseEvent {
	var out []sseEvent
	for _, block := range strings.Split(strings.TrimSpace(raw), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, ":") {
			continue // skip blanks and ping comments
		}
		var ev sseEvent
		for _, line := range strings.Split(block, "\n") {
			if v, ok := strings.CutPrefix(line, "event: "); ok {
				ev.name = v
			}
			if v, ok := strings.CutPrefix(line, "data: "); ok {
				ev.data = v
			}
		}
		if ev.name != "" {
			out = append(out, ev)
		}
	}
	return out
}

// collector is a running Serve connection backed by a fakeSSE, used to model a
// surface that connects, collects snapshots, and disconnects.
type collector struct {
	w      *fakeSSE
	cancel context.CancelFunc
}

func connectAndCollect(t *testing.T, b *Broadcaster[snapshot], auth *snapshot) *collector {
	t.Helper()
	w := newFakeSSE()
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("GET", "/stream", nil).WithContext(ctx)
	s, err := NewStream(w, r)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	hydrated := *auth // capture authoritative state at connect time
	go func() {
		_ = Serve(s, b,
			func(s *Stream) error { return s.Send("hydrate", hydrated) },
			func(s *Stream, ev snapshot) error { return s.Send("update", ev) },
		)
	}()
	return &collector{w: w, cancel: cancel}
}

func (c *collector) disconnect() { c.cancel() }

// latest returns the most recent snapshot the surface applied (hydrate or the
// last update), which is what it currently presents.
func (c *collector) latest() snapshot {
	events := parseSSE(c.w.String())
	var snap snapshot
	for _, ev := range events {
		var s snapshot
		if json.Unmarshal([]byte(ev.data), &s) == nil {
			snap = s
		}
	}
	return snap
}

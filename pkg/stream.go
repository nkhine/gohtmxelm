package gohtmxelm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrStreamingUnsupported is returned by NewStream when the ResponseWriter
// cannot flush, which means SSE is impossible on that connection.
var ErrStreamingUnsupported = errors.New("gohtmxelm: streaming unsupported")

// Stream is a server-sent-events writer bound to one request. It bundles the
// ResponseWriter, its Flusher, and the request context so handlers stop
// repeating the "type-assert the flusher, set the headers, flush after every
// write" ritual. Every write method flushes on success.
//
// Typical use:
//
//	s, err := gohtmxelm.NewStream(w, r)
//	if err != nil {
//		http.Error(w, err.Error(), http.StatusInternalServerError)
//		return
//	}
//	s.Send("hello", payload)
//	for {
//		select {
//		case ev := <-ch:
//			s.Send("update", ev)
//		case <-s.Done():
//			return
//		}
//	}
type Stream struct {
	w   http.ResponseWriter
	f   http.Flusher
	ctx context.Context
}

// NewStream sets the standard SSE headers and returns a Stream, or
// ErrStreamingUnsupported if the response cannot be flushed.
func NewStream(w http.ResponseWriter, r *http.Request) (*Stream, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, ErrStreamingUnsupported
	}
	PrepareSSE(w)
	return &Stream{w: w, f: f, ctx: r.Context()}, nil
}

// Send writes a named event with JSON-encoded data, then flushes.
func (s *Stream) Send(event string, data any) error {
	if err := WriteSSE(s.w, event, data); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

// PatchElements writes a Datastar element patch, then flushes.
func (s *Stream) PatchElements(elements string) error {
	if err := WriteDatastarPatchElements(s.w, elements); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

// PatchSignals writes a Datastar signal patch, then flushes. Unlike the
// lower-level WriteDatastarPatchSignals (which takes a pre-encoded string),
// this marshals any value to JSON for a consistent contract with Send.
func (s *Stream) PatchSignals(signals any) error {
	b, err := json.Marshal(signals)
	if err != nil {
		return err
	}
	if err := WriteDatastarPatchSignals(s.w, string(b)); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

// Ping writes an SSE comment heartbeat and flushes. Useful to keep idle
// connections alive through proxies that close quiet sockets.
func (s *Stream) Ping() error {
	if _, err := fmt.Fprint(s.w, ": ping\n\n"); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

// Done reports client disconnect / server shutdown via the request context.
func (s *Stream) Done() <-chan struct{} { return s.ctx.Done() }

// Serve runs the common SSE lifecycle against a Broadcaster: it subscribes,
// invokes hydrate once for the initial snapshot, then forwards every published
// value through each until the client disconnects. Unsubscribe is automatic.
// A nil hydrate or each is skipped. If hydrate or each returns an error the
// stream ends.
func Serve[T any](s *Stream, b *Broadcaster[T], hydrate func(*Stream) error, each func(*Stream, T) error) error {
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	if hydrate != nil {
		if err := hydrate(s); err != nil {
			return err
		}
	}
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				return nil
			}
			if each != nil {
				if err := each(s, v); err != nil {
					return err
				}
			}
		case <-s.Done():
			return nil
		}
	}
}

package edgedatastar

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"time"

	gohtmxelm "github.com/nkhine/gohtmxelm/pkg"
)

// ErrComplete is returned internally to stop the finite demo stream cleanly.
var ErrComplete = errors.New("edge datastar stream complete")

// Event is the authoritative state pushed to the Datastar island.
type Event struct {
	Seq     int
	Status  string
	Message string
	Last    bool
}

// Handler streams Datastar patches that are safe to serve behind API Gateway,
// Lambda response streaming, and the Starbase /api/* edge proxy.
func Handler() http.Handler {
	return HandlerWithDelay(450 * time.Millisecond)
}

// HandlerWithDelay exposes the tick delay for tests.
func HandlerWithDelay(delay time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stream, err := gohtmxelm.NewStream(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		events := gohtmxelm.NewBroadcaster[Event](8)
		err = gohtmxelm.Serve(stream, events,
			func(s *gohtmxelm.Stream) error {
				initial := Event{
					Seq:     0,
					Status:  "connected",
					Message: "hydrated by gohtmxelm.Serve",
				}
				if err := writePatch(s, initial); err != nil {
					return err
				}
				go publish(r.Context(), events, delay)
				return s.Ping()
			},
			func(s *gohtmxelm.Stream, ev Event) error {
				if err := writePatch(s, ev); err != nil {
					return err
				}
				if ev.Last {
					return ErrComplete
				}
				return nil
			},
		)
		if err != nil && !errors.Is(err, ErrComplete) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

func publish(ctx context.Context, events *gohtmxelm.Broadcaster[Event], delay time.Duration) {
	steps := []Event{
		{Seq: 1, Status: "edge", Message: "SSE entered through /api/* and Lambda streaming"},
		{Seq: 2, Status: "signals", Message: "datastar-patch-signals updated the live signal region"},
		{Seq: 3, Status: "morph", Message: "datastar-patch-elements morphed the panel by id"},
		{Seq: 4, Status: "rebound", Message: "bindings in the morphed element are live again", Last: true},
	}
	for _, ev := range steps {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			events.Publish(ev)
		}
	}
}

func writePatch(s *gohtmxelm.Stream, ev Event) error {
	if err := s.PatchElements(PanelHTML(ev)); err != nil {
		return err
	}
	return s.PatchSignals(Signals(ev))
}

// Signals returns the signal object patched by datastar-patch-signals.
func Signals(ev Event) map[string]any {
	return map[string]any{
		"edgeSeq":     ev.Seq,
		"edgeStatus":  ev.Status,
		"edgeMessage": ev.Message,
		"edgeRebind":  fmt.Sprintf("bound after morph #%d", ev.Seq),
		"edgeDone":    ev.Last,
	}
}

// PanelHTML is the element morph payload. It deliberately contains Datastar
// bindings so each morph must be re-scanned and re-bound by Datastar.
func PanelHTML(ev Event) string {
	status := html.EscapeString(ev.Status)
	statusClass := statusClass(ev.Status)
	message := html.EscapeString(ev.Message)
	return fmt.Sprintf(`<div id="edge-datastar-panel" class="edge-live-panel edge-live-%s">
	<div class="edge-live-head">
		<span class="edge-step">#%d</span>
		<strong data-text="'Status: ' + $edgeStatus">Status: %s</strong>
	</div>
	<p data-text="$edgeMessage">%s</p>
	<div class="edge-rebind-proof">
		<span>morph payload contains fresh bindings</span>
		<code data-text="$edgeRebind">waiting for signal patch</code>
	</div>
	<button type="button" class="edge-local-btn" data-on:click="$edgeClicks = $edgeClicks + 1">
		local Datastar click
	</button>
	<span class="edge-clicks" data-text="$edgeClicks + ' local clicks survived rebinding'">0 local clicks survived rebinding</span>
</div>`, statusClass, ev.Seq, status, message)
}

func statusClass(status string) string {
	switch status {
	case "connected", "edge", "signals", "morph", "rebound", "idle":
		return status
	default:
		return "unknown"
	}
}

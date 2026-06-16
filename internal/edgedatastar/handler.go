package edgedatastar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/nkhine/gohtmxelm"
)

// ErrComplete is returned internally to stop the finite demo stream cleanly.
var ErrComplete = errors.New("edge datastar stream complete")

// Event is the authoritative state pushed to the Datastar island.
type Event struct {
	Seq     int
	Cycle   int
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
	return handler(delay, 0)
}

// HandlerWithCycles exposes a finite loop count for tests and buffered adapters.
func HandlerWithCycles(delay time.Duration, cycles int) http.Handler {
	return handler(delay, cycles)
}

func handler(delay time.Duration, cycles int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if stopRequested(r) {
			w.WriteHeader(http.StatusNoContent)
			return
		}

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
					Cycle:   0,
					Status:  "connected",
					Message: "hydrated by gohtmxelm.Serve",
				}
				if err := writePatch(s, initial); err != nil {
					return err
				}
				go publish(r.Context(), events, delay, cycles)
				return s.Ping()
			},
			func(s *gohtmxelm.Stream, ev Event) error {
				if err := writePatch(s, ev); err != nil {
					return err
				}
				if cycles > 0 && ev.Last && ev.Cycle >= cycles {
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

func publish(ctx context.Context, events *gohtmxelm.Broadcaster[Event], delay time.Duration, cycles int) {
	seq := 1
	for cycle := 1; cycles == 0 || cycle <= cycles; cycle++ {
		steps := []Event{
			{Seq: seq, Cycle: cycle, Status: "connected", Message: fmt.Sprintf("loop %d opened EventSource", cycle)},
			{Seq: seq + 1, Cycle: cycle, Status: "edge", Message: "SSE entered through /api/* and Lambda streaming"},
			{Seq: seq + 2, Cycle: cycle, Status: "signals", Message: "datastar-patch-signals updated the live signal region"},
			{Seq: seq + 3, Cycle: cycle, Status: "morph", Message: "datastar-patch-elements morphed the panel by id"},
			{Seq: seq + 4, Cycle: cycle, Status: "rebound", Message: "bindings in the morphed element are live again", Last: true},
		}
		for _, ev := range steps {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
				events.Publish(ev)
			}
		}
		seq += len(steps)
	}
}

func stopRequested(r *http.Request) bool {
	raw := r.URL.Query().Get("datastar")
	if raw == "" {
		return false
	}
	var signals map[string]any
	if err := json.Unmarshal([]byte(raw), &signals); err != nil {
		return false
	}
	edgeRun, ok := signals["edgeRun"].(bool)
	return ok && !edgeRun
}

func writePatch(s *gohtmxelm.Stream, ev Event) error {
	if err := s.PatchElements(PanelHTML(ev)); err != nil {
		return err
	}
	return s.PatchSignals(Signals(ev))
}

// Signals returns the signal object patched by datastar-patch-signals.
func Signals(ev Event) map[string]any {
	node, recv, send, frame := trace(ev)
	return map[string]any{
		"edgeSeq":     ev.Seq,
		"edgeCycle":   ev.Cycle,
		"edgeStatus":  ev.Status,
		"edgeMessage": ev.Message,
		"edgeRebind":  fmt.Sprintf("bound after morph #%d", ev.Seq),
		"edgeDone":    ev.Last,
		"edgeNode":    node,
		"edgeRecv":    recv,
		"edgeSend":    send,
		"edgeFrame":   frame,
	}
}

func trace(ev Event) (node, recv, send, frame string) {
	switch ev.Status {
	case "connected":
		return "Datastar island", "start signal true", "GET /api/edge-datastar/stream", fmt.Sprintf("EventSource loop %d", ev.Cycle)
	case "edge":
		return "Starbase edge", "GET /api/edge-datastar/stream", "signed upstream GET to API Gateway", "edge request"
	case "signals":
		return "API Gateway", "SigV4 GET /api/edge-datastar/stream", "Lambda proxy invoke", "APIGatewayProxyRequest"
	case "morph":
		return "Go Lambda", "APIGatewayProxyRequest", "datastar-patch-elements #edge-datastar-panel", "element morph"
	case "rebound":
		return "Datastar island", "datastar-patch-elements + datastar-patch-signals", "morph DOM, patch signals, re-bind handlers", "browser apply"
	default:
		return "idle", "waiting", "waiting", "idle"
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

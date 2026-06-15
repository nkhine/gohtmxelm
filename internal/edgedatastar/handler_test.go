package edgedatastar

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHandlerStreamsDatastarElementAndSignalPatches(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/edge-datastar/stream", nil)
	rec := httptest.NewRecorder()

	HandlerWithCycles(time.Millisecond, 1).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"event: datastar-patch-elements\n",
		"event: datastar-patch-signals\n",
		`"edgeDone":true`,
		`"edgeCycle":1`,
		`"edgeNode":"Datastar island"`,
		`data: elements <div id="edge-datastar-panel"`,
		`data-text="$edgeRebind"`,
		`data-on:click="$edgeClicks = $edgeClicks + 1"`,
		": ping\n\n",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream missing %q in:\n%s", want, body)
		}
	}

	if got := strings.Count(body, "event: datastar-patch-elements\n"); got != 6 {
		t.Fatalf("element patches = %d, want 6", got)
	}
	if got := strings.Count(body, "event: datastar-patch-signals\n"); got != 6 {
		t.Fatalf("signal patches = %d, want 6", got)
	}
}

func TestHandlerStopsWhenRunSignalIsFalse(t *testing.T) {
	rawSignals := url.QueryEscape(`{"edgeRun":false}`)
	req := httptest.NewRequest("GET", "/api/edge-datastar/stream?datastar="+rawSignals, nil)
	rec := httptest.NewRecorder()

	HandlerWithCycles(time.Millisecond, 1).ServeHTTP(rec, req)

	if got := rec.Code; got != 204 {
		t.Fatalf("status = %d, want 204", got)
	}
	if body := rec.Body.String(); body != "" {
		t.Fatalf("body = %q, want empty", body)
	}
}

func TestPanelHTMLEscapesServerContent(t *testing.T) {
	html := PanelHTML(Event{
		Seq:     1,
		Status:  `"><script>`,
		Message: `<img src=x onerror=alert(1)>`,
	})
	for _, bad := range []string{
		`<script>`,
		`<img src=x onerror=alert(1)>`,
	} {
		if strings.Contains(html, bad) {
			t.Fatalf("PanelHTML leaked unescaped content %q in %s", bad, html)
		}
	}
}

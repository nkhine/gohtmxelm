package gohtmxelm

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserScript(t *testing.T) {
	script := BrowserScript(Options{
		AssetPath: "/assets/fusion",
		Sources: []Source{
			{URL: "/events", Events: []string{"store-change", "timer"}},
		},
		Debug: true,
	})

	for _, want := range []string{
		`src="/assets/fusion/gohtmxelm-broker.js"`,
		// data-sources holds HTML-escaped JSON; check the escaped fragments.
		`data-sources=`,
		`/events`,
		`store-change`,
		`timer`,
		`data-debug="true"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("BrowserScript() missing %q in %s", want, script)
		}
	}
}

func TestElmIsland(t *testing.T) {
	html, err := ElmIsland("counter", "Counter", map[string]any{"initial": 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`class="elm-island"`,
		`id="counter"`,
		`data-elm-module="Counter"`,
		`data-island-id="counter"`,
		`&#34;initial&#34;:1`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("ElmIsland() missing %q in %s", want, html)
		}
	}
}

func TestSSEHelpers(t *testing.T) {
	rec := httptest.NewRecorder()
	PrepareSSE(rec)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
	if err := WriteSSE(rec, "demo", map[string]string{"hello": "world"}); err != nil {
		t.Fatal(err)
	}
	if got := rec.Body.String(); !strings.Contains(got, "event: demo\n") || !strings.Contains(got, `"hello":"world"`) {
		t.Fatalf("unexpected SSE body: %q", got)
	}
}

func TestDatastarHelpers(t *testing.T) {
	var b strings.Builder
	if err := WriteDatastarPatchElements(&b, "<div id=\"x\">ok</div>"); err != nil {
		t.Fatal(err)
	}
	if err := WriteDatastarPatchSignals(&b, `{"count":1}`); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	for _, want := range []string{
		"event: datastar-patch-elements\n",
		"data: elements <div id=\"x\">ok</div>\n",
		"event: datastar-patch-signals\n",
		"data: signals {\"count\":1}\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Datastar helpers missing %q in %q", want, got)
		}
	}
}

package gohtmxelm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInteractionRoot(t *testing.T) {
	got := InteractionRoot(`x"><script>`)
	if !strings.Contains(got, `data-gohtmxelm-interactions-root`) {
		t.Fatalf("missing root marker: %s", got)
	}
	if strings.Contains(got, `<script>`) {
		t.Fatalf("root id was not escaped: %s", got)
	}
}

func TestInteractionScript(t *testing.T) {
	got := InteractionScript(Options{AssetPath: "/assets/"})
	if want := `src="/assets/gohtmxelm-interactions.js"`; !strings.Contains(got, want) {
		t.Fatalf("script path mismatch: got %s want %s", got, want)
	}
}

func TestInteractionAttrsEscape(t *testing.T) {
	got := InteractionOpenAttrs(`/x?a="b"`, `#result`)
	if strings.Contains(got, `"b"`) {
		t.Fatalf("attrs were not escaped: %s", got)
	}
	if !strings.Contains(got, `data-gohtmxelm-open=`) || !strings.Contains(got, `data-gohtmxelm-status=`) {
		t.Fatalf("missing expected attrs: %s", got)
	}
}

func TestMarshalInteractionEvent(t *testing.T) {
	got, err := MarshalInteractionEvent("#result", "accepted")
	if err != nil {
		t.Fatal(err)
	}
	var ev InteractionEvent
	if err := json.Unmarshal([]byte(got), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Target != "#result" || ev.Result != "accepted" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

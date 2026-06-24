package gohtmxelm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIMUIScript(t *testing.T) {
	got := IMUIScript(Options{
		AssetPath: "/assets/",
		Sources: []Source{
			{URL: "/events", Events: []string{"lattice-snapshot"}},
		},
		Debug: true,
	})
	for _, want := range []string{
		`src="/assets/gohtmxelm-imui.js"`,
		`data-sources=`,
		`/events`,
		`lattice-snapshot`,
		`data-debug="true"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("IMUIScript() missing %q in %s", want, got)
		}
	}
}

func TestCanvasIsland(t *testing.T) {
	got, err := CanvasIsland("lattice", "LatticeTool", map[string]any{"snap": true}, CanvasOptions{
		Width:      640,
		Height:     360,
		CommandURL: "/api/lattice/commands",
		Events:     []string{"lattice-snapshot"},
		Class:      "lattice-canvas",
		Label:      `Lattice "construction"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<canvas`,
		`class="gohtmxelm-imui lattice-canvas"`,
		`id="lattice"`,
		`data-gohtmxelm-imui-module="LatticeTool"`,
		`data-gohtmxelm-imui-id="lattice"`,
		`data-command-url="/api/lattice/commands"`,
		`lattice-snapshot`,
		`tabindex="0"`,
		`role="img"`,
		`aria-label="Lattice &#34;construction&#34;"`,
		`width="640"`,
		`height="360"`,
		`&#34;snap&#34;:true`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("CanvasIsland() missing %q in %s", want, got)
		}
	}
}

func TestMarshalIMUICommandEvent(t *testing.T) {
	got, err := MarshalIMUICommandEvent("lattice", map[string]any{"type": "add-cover"})
	if err != nil {
		t.Fatal(err)
	}
	var ev IMUICommandEvent
	if err := json.Unmarshal([]byte(got), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.IslandID != "lattice" {
		t.Fatalf("IslandID = %q", ev.IslandID)
	}
	cmd, ok := ev.Command.(map[string]any)
	if !ok || cmd["type"] != "add-cover" {
		t.Fatalf("unexpected command: %#v", ev.Command)
	}
}

func TestIMUIRuntimeEmbedded(t *testing.T) {
	if _, err := runtimeFS.ReadFile("runtime/gohtmxelm-imui.js"); err != nil {
		t.Fatalf("read IMUI runtime: %v", err)
	}
}

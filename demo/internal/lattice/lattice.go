package lattice

import (
	"fmt"
	"math"
	"sync"

	"github.com/nkhine/gohtmxelm"
)

// Node is a visual lattice point. X/Y are normalized canvas coordinates.
type Node struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Level    int     `json:"level"`
	Selected bool    `json:"selected"`
}

// Edge is a cover relation in the rendered Hasse diagram.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// State is the canonical server-owned lattice snapshot broadcast to every UI.
type State struct {
	Seq        uint64 `json:"seq"`
	Nodes      []Node `json:"nodes"`
	Edges      []Edge `json:"edges"`
	Selected   string `json:"selected"`
	LastAction string `json:"lastAction"`
	LastSource string `json:"lastSource"`
	Valid      bool   `json:"valid"`
}

// Command is the shared command shape accepted from HTMX, Elm, and IMUI.
type Command struct {
	Action string `json:"action"`
	Node   string `json:"node,omitempty"`
	Source string `json:"source,omitempty"`
}

// Model owns the lattice state and fans snapshots to subscribers.
type Model struct {
	mu       sync.Mutex
	bc       *gohtmxelm.Broadcaster[State]
	seq      uint64
	nodes    []Node
	edges    []Edge
	selected string
	last     string
	source   string
}

// New returns a small valid lattice labelled with Vim movement keys:
// h=left, j=down, k=up, l=right.
func New() *Model {
	m := &Model{bc: gohtmxelm.NewBroadcaster[State](16)}
	m.resetLocked("go")
	return m
}

func (m *Model) Events() *gohtmxelm.Broadcaster[State] { return m.bc }

func (m *Model) Snapshot() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stateLocked()
}

func (m *Model) Apply(cmd Command) State {
	m.mu.Lock()
	defer m.mu.Unlock()

	source := cleanSource(cmd.Source)
	action := cmd.Action
	switch action {
	case "select-node":
		if m.hasNodeLocked(cmd.Node) {
			m.selected = cmd.Node
			m.last = fmt.Sprintf("selected %s", cmd.Node)
		}
	case "add-node":
		m.addNodeLocked()
		m.last = "added middle node"
	case "promote":
		if m.selected != "" && m.selected != "k" && m.selected != "j" {
			for i := range m.nodes {
				if m.nodes[i].ID == m.selected && m.nodes[i].Level < 2 {
					m.nodes[i].Level++
					m.nodes[i].Y = math.Max(0.18, m.nodes[i].Y-0.18)
					m.last = fmt.Sprintf("promoted %s", m.selected)
				}
			}
		}
	case "reset":
		m.resetLocked(source)
		state := m.stateLocked()
		m.bc.Publish(state)
		return state
	default:
		m.last = "ignored unknown command"
	}
	m.source = source
	m.seq++
	m.layoutMiddleLocked()
	state := m.stateLocked()
	m.bc.Publish(state)
	return state
}

func (m *Model) addNodeLocked() {
	id := fmt.Sprintf("n%d", len(m.nodes)-1)
	m.nodes = append(m.nodes, Node{ID: id, Label: id, Level: 1, X: 0.5, Y: 0.5})
	m.edges = append(m.edges, Edge{From: "j", To: id}, Edge{From: id, To: "k"})
	m.selected = id
}

func (m *Model) resetLocked(source string) {
	m.seq++
	m.nodes = []Node{
		{ID: "j", Label: "j", X: 0.5, Y: 0.86, Level: 0},
		{ID: "h", Label: "h", X: 0.32, Y: 0.52, Level: 1},
		{ID: "l", Label: "l", X: 0.68, Y: 0.52, Level: 1},
		{ID: "k", Label: "k", X: 0.5, Y: 0.18, Level: 2},
	}
	m.edges = []Edge{
		{From: "j", To: "h"},
		{From: "j", To: "l"},
		{From: "h", To: "k"},
		{From: "l", To: "k"},
	}
	m.selected = "h"
	m.last = "reset lattice"
	m.source = source
	m.layoutMiddleLocked()
}

func (m *Model) layoutMiddleLocked() {
	var mids []int
	for i, n := range m.nodes {
		if n.ID != "j" && n.ID != "k" {
			mids = append(mids, i)
		}
	}
	for j, i := range mids {
		x := float64(j+1) / float64(len(mids)+1)
		m.nodes[i].X = x
		if m.nodes[i].Level <= 1 {
			m.nodes[i].Y = 0.52
		}
	}
}

func (m *Model) hasNodeLocked(id string) bool {
	for _, n := range m.nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func (m *Model) stateLocked() State {
	nodes := append([]Node(nil), m.nodes...)
	for i := range nodes {
		nodes[i].Selected = nodes[i].ID == m.selected
	}
	return State{
		Seq:        m.seq,
		Nodes:      nodes,
		Edges:      append([]Edge(nil), m.edges...),
		Selected:   m.selected,
		LastAction: m.last,
		LastSource: m.source,
		Valid:      true,
	}
}

func cleanSource(s string) string {
	switch s {
	case "htmx", "datastar", "elm", "imui", "go":
		return s
	default:
		return "unknown"
	}
}

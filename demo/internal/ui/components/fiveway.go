package components

import (
	"github.com/a-h/templ"

	"github.com/nkhine/gohtmxelm"
)

func fiveWayCanvas() templ.Component {
	html, _ := gohtmxelm.CanvasIsland("fiveway-canvas", "FiveWayLattice", map[string]any{
		"accent": "#0ea5e9",
	}, gohtmxelm.CanvasOptions{
		CommandURL: "/api/lattice/command",
		Events:     []string{"lattice-state"},
		Class:      "fiveway-canvas",
		Label:      "Immediate-mode lattice construction canvas",
	})
	return templ.Raw(html)
}

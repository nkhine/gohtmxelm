package ui

import (
	"github.com/a-h/templ"

	gohtmxelm "github.com/nkhine/gohtmxelm/pkg"
)

// brokerScript is the configured gohtmxelm broker <script> tag, rendered from
// the package's own BrowserScript helper so the demo exercises the public
// contract. The demo opens two SSE sources: the store stream and the stopwatch
// state stream. Their events reach Elm islands as generic SSE_EVENT envelopes.
var brokerScript = templ.Raw(gohtmxelm.BrowserScript(gohtmxelm.Options{
	AssetPath: "/gohtmxelm",
	// One multiplexed source carries every broker event. Browsers cap HTTP/1.1
	// connections at ~6 per host, and each EventSource holds one open — so a
	// separate stream per domain would exhaust the pool once a few examples
	// share a page. Multiplexing keeps the broker to a single connection.
	Sources: []gohtmxelm.Source{
		{URL: "/api/stream", Events: []string{
			"store-hydrate", "store-change", "stopwatch-state", "statement-range-change",
			"sim-frame", "auth-presence",
		}},
	},
}))

var interactionScript = templ.Raw(gohtmxelm.InteractionScript(gohtmxelm.Options{
	AssetPath: "/gohtmxelm",
}))

var interactionRoot = templ.Raw(gohtmxelm.InteractionRoot(""))

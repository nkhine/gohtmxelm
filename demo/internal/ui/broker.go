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
	Sources: []gohtmxelm.Source{
		{URL: "/api/events", Events: []string{"store-hydrate", "store-change"}},
		{URL: "/api/stopwatch/events", Events: []string{"stopwatch-state"}},
	},
}))

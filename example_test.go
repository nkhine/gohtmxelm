package gohtmxelm_test

import (
	"fmt"
	"net/http"

	"github.com/nkhine/gohtmxelm"
)

// Mount the embedded broker runtime and render the boot script for pages that
// use Elm islands and a server-sent-events stream.
func ExampleNew() {
	kit := gohtmxelm.New(gohtmxelm.Options{
		AssetPath: "/gohtmxelm",
		Sources: []gohtmxelm.Source{
			{URL: "/api/events", Events: []string{"store-change"}},
		},
	})

	mux := http.NewServeMux()
	mux.Handle("/gohtmxelm/", http.StripPrefix("/gohtmxelm/", kit.Assets()))

	fmt.Println(kit.BrowserScript())
	// Output:
	// <script defer src="/gohtmxelm/gohtmxelm-broker.js" data-sources="[{&#34;url&#34;:&#34;/api/events&#34;,&#34;events&#34;:[&#34;store-change&#34;]}]"></script>
}

// Render an Elm island mount point. The broker looks the module up on
// window.Elm by name and hydrates it with the JSON-encoded props.
func ExampleElmIsland() {
	html, err := gohtmxelm.ElmIsland("cart", "AppA", map[string]any{"count": 3})
	if err != nil {
		panic(err)
	}
	fmt.Println(html)
	// Output:
	// <div class="elm-island" id="cart" data-elm-module="AppA" data-island-id="cart" data-props="{&#34;count&#34;:3}"></div>
}

// Stream server-sent events to the browser broker. Send flushes after every
// write, and Done reports client disconnect.
func ExampleStream() {
	handler := func(w http.ResponseWriter, r *http.Request) {
		s, err := gohtmxelm.NewStream(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.Send("store-change", map[string]string{"key": "total", "value": "42"}); err != nil {
			return
		}
		<-s.Done()
	}
	_ = handler
}

// Broadcaster fans one published value out to every subscriber without the
// publisher ever blocking on a slow consumer.
func ExampleBroadcaster() {
	b := gohtmxelm.NewBroadcaster[string](16)
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	b.Publish("hello")
	fmt.Println(<-ch)
	// Output:
	// hello
}

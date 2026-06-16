package gohtmxelm_test

import (
	"fmt"
	"os"
	"strings"

	"github.com/nkhine/gohtmxelm"
)

// Merge application props with a standard locale payload for an Elm island.
func ExampleLocalizedProps() {
	props, err := gohtmxelm.LocalizedProps(
		map[string]any{"count": 3},
		gohtmxelm.LocaleProps{Locale: "en-GB", Currency: "GBP"},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(props)
	// Output:
	// map[count:3 currency:GBP locale:en-GB]
}

// Serialise the attributes for a button that opens a server-rendered
// interaction fragment and reports its result into a status element.
func ExampleInteractionOpenAttrs() {
	fmt.Printf("<button%s>Delete</button>\n",
		gohtmxelm.InteractionOpenAttrs("/api/interactions/confirm", "#status"))
	// Output:
	// <button data-gohtmxelm-open="/api/interactions/confirm" data-gohtmxelm-status="#status">Delete</button>
}

// Write a Datastar signal patch to an SSE response.
func ExampleWriteDatastarPatchSignals() {
	_ = gohtmxelm.WriteDatastarPatchSignals(os.Stdout, `{"count":3}`)
	// Output:
	// event: datastar-patch-signals
	// data: signals {"count":3}
}

// Vendor the canonical Elm-side contract into a project's Elm source directory.
func ExampleElmBrokerPort() {
	src := gohtmxelm.ElmBrokerPort()
	fmt.Println(strings.SplitN(src, "\n", 2)[0])
	// Output:
	// port module BrokerPort exposing
}

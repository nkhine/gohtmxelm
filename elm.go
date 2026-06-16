package gohtmxelm

import (
	_ "embed"
	"net/http"
	"strings"
)

// elmBrokerPort is the canonical Elm-side contract every island imports as
// BrokerPort. It is the third peer of the wire contract alongside Envelope
// (Go) and the embedded broker runtime (JavaScript); all three stamp and
// validate ProtocolVersion. It is embedded so importers can vendor a
// known-good copy that matches the broker they are running, instead of
// hand-copying a file that silently drifts.
//
//go:embed elm/BrokerPort.elm
var elmBrokerPort string

// ElmBrokerPort returns the canonical BrokerPort.elm source. Write it into your
// project's Elm source directory (the module name is "BrokerPort") so islands
// can `import BrokerPort`. The returned source matches ProtocolVersion, so
// re-vendoring after a library upgrade is how you stay in sync with the broker.
func ElmBrokerPort() string {
	return elmBrokerPort
}

// ElmContractHandler serves the embedded BrokerPort.elm source as plain text.
// It is handy for a `gohtmxelm doctor`-style endpoint or for tooling that
// fetches the contract over HTTP; most projects vendor ElmBrokerPort() at build
// time instead.
func ElmContractHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = strings.NewReader(elmBrokerPort).WriteTo(w)
	})
}

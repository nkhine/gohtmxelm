package gohtmxelm

import (
	"regexp"
	"strconv"
	"testing"
)

// The wire contract lives in three languages: Go (ProtocolVersion), the broker
// runtime (JavaScript const PROTOCOL_VERSION), and the Elm island contract
// (BrokerPort.protocolVersion). Nothing in the type systems forces them to
// agree, so this test is the single guard that keeps the three copies in sync.
// If you bump ProtocolVersion, bump the other two and this test goes green.

func TestProtocolVersionMatchesRuntimeJS(t *testing.T) {
	js, err := runtimeFS.ReadFile("runtime/gohtmxelm-broker.js")
	if err != nil {
		t.Fatalf("read broker runtime: %v", err)
	}
	got := extractInt(t, `(?m)^const PROTOCOL_VERSION\s*=\s*(\d+)`, string(js), "broker.js PROTOCOL_VERSION")
	if got != ProtocolVersion {
		t.Errorf("broker.js PROTOCOL_VERSION = %d, want ProtocolVersion = %d", got, ProtocolVersion)
	}
}

func TestProtocolVersionMatchesElmContract(t *testing.T) {
	got := extractInt(t, `protocolVersion\s*=\s*\n?\s*(\d+)`, elmBrokerPort, "BrokerPort.protocolVersion")
	if got != ProtocolVersion {
		t.Errorf("BrokerPort.protocolVersion = %d, want ProtocolVersion = %d", got, ProtocolVersion)
	}
}

func extractInt(t *testing.T, pattern, src, label string) int {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("could not find %s using %q", label, pattern)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("parse %s: %v", label, err)
	}
	return n
}

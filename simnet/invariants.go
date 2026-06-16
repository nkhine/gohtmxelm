package simnet

import (
	"fmt"
	"sort"
)

// View is one surface's observable state: the data it has applied and the
// authoritative version that data reflects. HTMX tables, Datastar signals, and
// Elm islands are all Views in this model — whatever the transport, a surface
// is "the keys it currently shows and how fresh they are".
type View struct {
	// Label identifies the surface in violation reports (e.g. "surface-2").
	Label string
	// Data is the key/value state the surface currently presents.
	Data map[string]string
	// Version is the authoritative version Data reflects. It must never move
	// backwards on a single surface (see CheckMonotonic).
	Version int
}

// Authoritative is the server-owned source of truth that every View must
// converge to once the system quiesces.
type Authoritative struct {
	Data    map[string]string
	Version int
}

// CheckConvergence is THE contract of the pattern: after writes stop and the
// network settles, every connected surface must present exactly the
// authoritative state. A non-nil error names the first divergent surface and
// the mismatching key — that error is what a simnet Violation and a real-code
// test both report, so the model and the implementation are held to the same
// bar.
//
// It is deliberately strict about both data and version: a surface showing the
// right values but a stale version has not truly converged (its next in-order
// delta would be misapplied), and a surface on the right version with wrong
// data has lost an update.
func CheckConvergence(auth Authoritative, views []View) error {
	for _, v := range views {
		if v.Version != auth.Version {
			return fmt.Errorf("%s diverged: version %d, want authoritative %d",
				label(v.Label), v.Version, auth.Version)
		}
		if len(v.Data) != len(auth.Data) {
			return fmt.Errorf("%s diverged: holds %d keys, authoritative has %d",
				label(v.Label), len(v.Data), len(auth.Data))
		}
		for _, k := range sortedKeys(auth.Data) {
			got, ok := v.Data[k]
			if !ok {
				return fmt.Errorf("%s diverged: missing key %q", label(v.Label), k)
			}
			if got != auth.Data[k] {
				return fmt.Errorf("%s diverged: key %q = %q, authoritative = %q",
					label(v.Label), k, got, auth.Data[k])
			}
		}
	}
	return nil
}

// CheckMonotonic verifies that the versions a single surface applied, in the
// order it applied them, never decreased. A surface that regresses to an older
// version has applied a stale or reordered event over a fresher one — the
// ordering half of the contract. The sequence is the surface's applied-version
// history; equal consecutive versions (idempotent re-applies) are allowed.
func CheckMonotonic(label string, appliedVersions []int) error {
	for i := 1; i < len(appliedVersions); i++ {
		if appliedVersions[i] < appliedVersions[i-1] {
			return fmt.Errorf("%s applied version %d after %d: ordering violated at step %d",
				labelOr(label), appliedVersions[i], appliedVersions[i-1], i)
		}
	}
	return nil
}

func label(s string) string {
	if s == "" {
		return "surface"
	}
	return s
}

func labelOr(s string) string { return label(s) }

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

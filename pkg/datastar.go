package gohtmxelm

import (
	"fmt"
	"io"
	"strings"
)

// WriteDatastarPatchElements writes a Datastar element patch event to an SSE
// response. The supplied HTML should include stable ids for Datastar targets.
func WriteDatastarPatchElements(w io.Writer, elements string) error {
	if _, err := fmt.Fprint(w, "event: datastar-patch-elements\n"); err != nil {
		return err
	}
	for _, line := range strings.Split(elements, "\n") {
		if _, err := fmt.Fprintf(w, "data: elements %s\n", line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "\n")
	return err
}

// WriteDatastarPatchSignals writes a Datastar signal patch event to an SSE
// response. The signals value must be a JSON object string.
func WriteDatastarPatchSignals(w io.Writer, signals string) error {
	_, err := fmt.Fprintf(w, "event: datastar-patch-signals\ndata: signals %s\n\n", signals)
	return err
}

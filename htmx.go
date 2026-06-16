package gohtmxelm

import "net/http"

// Trigger sets HX-Trigger so HTMX clients can react to a server-side event.
func Trigger(w http.ResponseWriter, event string) {
	w.Header().Add("HX-Trigger", event)
}

// NoContent writes a normal empty HTMX response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

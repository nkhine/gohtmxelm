package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"elm-htmx-templ-demo/internal/store"
	"elm-htmx-templ-demo/templates"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	kv := store.New()
	kv.Set("greeting", "hello world")
	kv.Set("status", "running")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		if err := templates.Page().Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	r.Get("/message", func(w http.ResponseWriter, r *http.Request) {
		if err := templates.ServerMessage("Hello from Go via HTMX").Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Store fragment — rendered by HTMX on load and on every store-refresh event.
	r.Get("/api/store/fragment", func(w http.ResponseWriter, r *http.Request) {
		if err := templates.StoreEntries(kv.All()).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Set a key/value — accepts application/x-www-form-urlencoded (HTMX form)
	// or application/json (broker.js sync).
	r.Post("/api/store", func(w http.ResponseWriter, r *http.Request) {
		key, value, err := parseStoreBody(r)
		if err != nil || key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}
		kv.Set(key, value)
		w.WriteHeader(http.StatusNoContent)
	})

	// SSE stream — pushes store-change events to every connected browser tab.
	// WriteTimeout is disabled on the server so long-lived SSE connections are
	// not killed mid-stream; context cancellation handles client disconnects.
	r.Get("/api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := kv.Subscribe()
		defer kv.Unsubscribe(ch)

		// Hydrate the client with the full current state before streaming deltas.
		for k, v := range kv.All() {
			writeSSE(w, "store-change", map[string]string{"key": k, "value": v})
		}
		flusher.Flush()

		for {
			select {
			case e, ok := <-ch:
				if !ok {
					return
				}
				writeSSE(w, "store-change", map[string]string{"key": e.Key, "value": e.Value})
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0, // disabled — SSE connections are long-lived
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("server started", "addr", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}

// parseStoreBody reads key/value from either a JSON body or form data.
func parseStoreBody(r *http.Request) (key, value string, err error) {
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		err = json.NewDecoder(r.Body).Decode(&body)
		return body.Key, body.Value, err
	}
	return r.FormValue("key"), r.FormValue("value"), nil
}

func writeSSE(w http.ResponseWriter, event string, data any) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

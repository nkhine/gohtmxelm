package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/nkhine/gohtmxelm/internal/edgedatastar"
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, req events.APIGatewayProxyRequest) (*events.APIGatewayProxyStreamingResponse, error) {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		target := req.Path
		if target == "" {
			target = "/api/edge-datastar/stream"
		}
		if qs := queryString(req.QueryStringParameters); qs != "" {
			target += "?" + qs
		}

		httpReq := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}

		edgedatastar.Handler().ServeHTTP(&pipeResponseWriter{
			header: make(http.Header),
			w:      pw,
			status: http.StatusOK,
		}, httpReq)
	}()

	return &events.APIGatewayProxyStreamingResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":      "text/event-stream",
			"Cache-Control":     "no-cache, no-transform",
			"Connection":        "keep-alive",
			"X-Accel-Buffering": "no",
		},
		Body: pr,
	}, nil
}

func queryString(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	q := make(url.Values, len(values))
	for k, v := range values {
		q.Set(k, v)
	}
	return q.Encode()
}

type pipeResponseWriter struct {
	header http.Header
	w      io.Writer
	status int
}

func (p *pipeResponseWriter) Header() http.Header { return p.header }

func (p *pipeResponseWriter) WriteHeader(status int) { p.status = status }

func (p *pipeResponseWriter) Write(b []byte) (int, error) { return p.w.Write(b) }

func (p *pipeResponseWriter) Flush() {}

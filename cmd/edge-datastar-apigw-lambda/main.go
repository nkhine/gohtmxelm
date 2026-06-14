package main

import (
	"context"
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

func handle(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
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

	rec := httptest.NewRecorder()
	edgedatastar.Handler().ServeHTTP(rec, httpReq)

	headers := map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache, no-transform",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	}
	for k, values := range rec.Result().Header {
		if len(values) > 0 {
			headers[k] = values[0]
		}
	}

	return events.APIGatewayProxyResponse{
		StatusCode: rec.Code,
		Headers:    headers,
		Body:       rec.Body.String(),
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

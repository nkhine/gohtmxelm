# 11. Datastar over SSE through the edge

Datastar does not need a websocket for server push. Its patch protocol is
ordinary Server-Sent Events:

```text
event: datastar-patch-elements
data: elements <div id="edge-datastar-panel">...</div>

event: datastar-patch-signals
data: signals {"edgeSeq":1,"edgeStatus":"edge"}
```

That means the edge path has one hard requirement: it must stream bytes as they
are written. If an intermediary buffers the Lambda response, Datastar receives
all patches only when the function exits, which defeats the purpose.

## Demo shape

The reference demo adds:

- `/examples/edge-datastar`: a Datastar signal region with a morph target.
- `/api/edge-datastar/stream`: a `gohtmxelm.Serve` handler that hydrates, then
  loops through `datastar-patch-elements` and `datastar-patch-signals` frames.
  The demo exposes Start/Stop controls by sending the `edgeRun` Datastar signal
  with the GET request.
- `cmd/edge-datastar-apigw-lambda`: a floci/API Gateway-compatible Lambda
  adapter returning `events.APIGatewayProxyResponse` with an SSE body.
- `cmd/edge-datastar-lambda`: a Go Lambda response-streaming adapter returning
  `events.APIGatewayProxyStreamingResponse`.
- `infra/`: a floci/Pulumi local stack with API Gateway REST API stage `api`
  and a Lambda integration URI ending in `/invocations`.

The morphed element deliberately includes fresh Datastar bindings:

```html
<code data-text="$edgeRebind">waiting for signal patch</code>
<button data-on:click="$edgeClicks = $edgeClicks + 1">...</button>
```

The server sends the element patch first and the signal patch second. If the
`data-text` value inside the newly morphed element updates, Datastar applied the
patch and re-bound the new subtree. Clicking the button after morphs confirms
local `data-on:*` handlers were re-bound too.

The live browser demo keeps the stream open and repeats the edge path so the
terminal trace is continuously updated. Stop flips `edgeRun` to false and issues
the same `@get('/api/edge-datastar/stream')` action; Datastar's default request
cancellation aborts the active stream, and the new request returns `204 No
Content`. Start flips `edgeRun` back to true and opens a fresh stream.

## Local floci deployment

Start Starbase/floci if it is not already running, package the Lambda, and deploy
the local stack:

```sh
make edge-infra-up
```

Inspect the local invoke URL and Starbase origin value:

```sh
make edge-infra-output
```

Important outputs:

```text
edge:sameOriginPath      /api/edge-datastar/stream
edge:localInvokeUrl      http://localhost:4566/restapis/<id>/api/_user_request_/edge-datastar/stream
starbase:SUBSPACE_ORIGIN http://localhost:4566/restapis/<id>/api/_user_request_
```

The Starbase Worker forwards `/api/*` paths as-is. With
`SUBSPACE_ORIGIN` set to the exported origin, a browser request to
`/api/edge-datastar/stream` maps to the API Gateway stage `api` and resource
`/edge-datastar/stream`.

Direct calls to floci API Gateway must be SigV4-signed. Unsigned requests can
return a misleading `Invalid API id specified` response even when the API exists:

```sh
API_URL=$(cd infra && pulumi stack output edge:localInvokeUrl --stack local)
curl --aws-sigv4 'aws:amz:eu-west-1:execute-api' \
  --user 'test:test' \
  -N "$API_URL"
```

The floci local stack uses the `edge-datastar-apigw-lambda` adapter because this
floci build returns headers but drops the body for
`APIGatewayProxyStreamingResponse` through REST API Gateway. The adapter still
runs the same `gohtmxelm.Serve` handler and returns the same Datastar SSE frames;
it buffers a one-cycle demo stream into the Lambda proxy response body so
floci's API Gateway data plane can return it.

For AWS response streaming, package `cmd/edge-datastar-lambda`, use the
`/response-streaming-invocations` integration URI, and set API Gateway's
integration `responseTransferMode` to `STREAM`.

## Local app smoke test

For a quick browser test without deploying Lambda:

```sh
make dev
```

`make dev` and `make watch` do not start floci, API Gateway, or Lambda. They
only run the local Go demo app. Use `make edge-infra-up` for the local AWS edge
stack.

Open:

```text
http://localhost:8091/examples/edge-datastar
```

That path uses the same shared handler directly in the demo server. The Lambda
and API Gateway path uses the same handler through `cmd/edge-datastar-lambda`.

## Production notes

- Use Lambda response streaming with API Gateway REST integrations configured
  with `responseTransferMode=STREAM`.
- Preserve `Content-Type: text/event-stream`, `Cache-Control: no-cache,
  no-transform`, and disable proxy buffering where your edge supports it.
- Keep the browser path same-origin (`/api/*`) so cookies, CSP, and CORS remain
  simple.
- Use SSE for Datastar patches unless you need bidirectional low-latency client
  messages. Datastar's server patch protocol is already SSE-native.

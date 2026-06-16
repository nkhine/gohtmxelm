# 1. Networking foundations

> If you only learn one thing from this whole project, make it this: **a web
> framework is a thin layer over bytes moving through a socket.** Once you can
> see the bytes, frameworks stop being magic and start being tools.

This document builds up from the bottom: a TCP connection, then HTTP/1.1 framing
on top of it, then *chunked transfer encoding*, then *Server-Sent Events*. Every
real-time feature in the reference demo rides on that stack, so we'll trace it concretely.

---

## 1.1 TCP: a reliable, ordered byte pipe

When your browser talks to this server, the first thing that happens — below all
the HTTP — is a **TCP connection**. TCP gives you four guarantees that everything
above it depends on:

1. **Reliable** — bytes you send arrive, or you find out the connection broke.
2. **Ordered** — bytes arrive in the order you sent them.
3. **Stream-oriented** — there are *no messages*, only a continuous stream of
   bytes. TCP does not know where one "thing" ends and the next begins. That
   boundary problem is left to the layer above (HTTP), and it matters enormously
   for streaming — keep it in mind.
4. **Bidirectional** — both ends can send at any time.

TCP does *not* give you: message boundaries, encryption (that's TLS), or any
notion of "request" and "response." Those are HTTP's inventions.

A key consequence: because TCP is just a byte stream, the server can keep a
connection **open** and keep writing to it for minutes or hours. The browser
will keep receiving. This is the entire basis of "real-time" web — there is no
special "push" protocol needed at the TCP level; you simply don't close the
connection. SSE, which we use everywhere here, is exactly that idea dressed up
in a tiny convention.

---

## 1.2 HTTP/1.1: framing requests and responses over the byte stream

HTTP/1.1 is a *text protocol* layered on the TCP byte stream. A request is:

```
GET /api/stream HTTP/1.1\r\n
Host: localhost:8091\r\n
Accept: text/event-stream\r\n
\r\n
```

Note the `\r\n` (carriage-return + line-feed) line endings and the **blank line**
(`\r\n` on its own) that signals "headers are done." That blank line is a framing
device: it tells the receiver where the headers stop and the body — if any —
begins.

A response looks similar:

```
HTTP/1.1 200 OK\r\n
Content-Type: text/event-stream\r\n
\r\n
...body bytes...
```

Now the crucial question for any response: **how does the receiver know when the
body is finished?** TCP won't tell it — TCP has no message boundaries. HTTP has
exactly two answers:

### Answer A: `Content-Length`

```
Content-Type: text/html\r\n
Content-Length: 1024\r\n
\r\n
<exactly 1024 bytes>
```

The server says "the body is 1024 bytes." The client reads 1024 bytes and knows
it's done. Simple — but it requires the server to **know the full size up front**.
For a streaming response that could go on forever, you can't. So `Content-Length`
is useless for SSE.

### Answer B: `Transfer-Encoding: chunked`

This is the one that matters for us. Instead of declaring a total size, the
server sends the body as a sequence of **chunks**, each prefixed by its length in
hexadecimal:

```
HTTP/1.1 200 OK\r\n
Content-Type: text/event-stream\r\n
Transfer-Encoding: chunked\r\n
\r\n
1a\r\n
event: store-change\ndata: {}\n\n\r\n
0\r\n
\r\n
```

Read that carefully:

- `1a\r\n` — the next chunk is `0x1a` = 26 bytes long.
- then 26 bytes of payload, then `\r\n`.
- `0\r\n\r\n` — a chunk of length **zero**. This is the **terminator**. It means
  "the body is complete; there are no more chunks."

The chunked terminator (`0\r\n\r\n`) is the hero — and the villain — of this
project's most instructive bug. Hold that thought.

Chunked encoding is what lets a server stream an *unknown-length* response: it
can keep emitting chunks for as long as it likes, and only sends the zero-chunk
when it's genuinely done (or never, for an infinite stream that ends only when
the connection closes).

In Go, you almost never write chunk headers by hand. When you write to an
`http.ResponseWriter` without setting `Content-Length` and then call
`Flusher.Flush()`, the standard library switches to chunked encoding and writes
the framing for you. You'll see exactly that in [`demo/main.go`](../demo/main.go).

---

## 1.3 Server-Sent Events: a convention, not a new protocol

Here is the thing that surprises people: **SSE is not a new protocol.** It is an
ordinary HTTP response with:

- the header `Content-Type: text/event-stream`,
- a body that is never finished (no `Content-Length`, so chunked encoding), and
- a tiny text format for the events.

The format is almost insultingly simple. Each event is a few lines, terminated by
a **blank line**:

```
event: store-change
data: {"key":"message","value":"hi","seq":7}

event: store-hydrate
data: {"key":"status","value":"running"}

```

Rules:
- `event:` names the event type (optional; defaults to `message`).
- `data:` is the payload. Multiple `data:` lines are concatenated with newlines.
- A blank line dispatches the event to the browser.
- Lines beginning with `:` are comments (often used as keep-alive "pings").

That's the entire protocol. `gohtmxelm` wraps the formatting so handlers do not
repeat it. From [`demo/main.go`](../demo/main.go):

```go
func writeSSE(w http.ResponseWriter, event string, data any) {
	_ = gohtmxelm.WriteSSE(w, event, data)
}
```

The browser side is equally simple. The `EventSource` API opens the connection
and fires events as data arrives. The generic broker
([`runtime/gohtmxelm-broker.js`](../runtime/gohtmxelm-broker.js)) opens
every source listed in `data-sources` and forwards each named event to islands:

```js
const source = new EventSource(s.url);
s.names.forEach((name) =>
  source.addEventListener(name, (event) => this.forwardSSE(name, event))
);
```

### Why SSE and not WebSockets?

WebSockets give you a *bidirectional* channel. SSE gives you *server → client
only*. For the reference demo that's a perfect fit, and it's worth understanding why the
weaker tool is the better choice:

- **Our writes go up via ordinary HTTP requests** (form POSTs, `fetch`). We don't
  need a second uplink channel. The browser already has the best uplink there is:
  a normal request.
- **SSE is just HTTP.** It works through every proxy, every CDN, every corporate
  firewall that already passes HTTP. WebSockets use an `Upgrade` handshake that
  many intermediaries mishandle.
- **SSE auto-reconnects for free.** The `EventSource` API reconnects on drop and
  can resume from a `Last-Event-ID`. With WebSockets you write reconnection logic
  yourself.
- **SSE is trivial to produce.** No framing library, no ping/pong frames — just
  `fmt.Fprintf` and a `Flush()`.

The lesson generalizes: **pick the least-powerful tool that solves the problem.**
A bidirectional socket is more capability than a fan-out notification stream
needs, and that extra capability is pure cost — more code, more failure modes,
worse infrastructure compatibility.

### The flush is mandatory

A streaming handler must call `Flush()` after each event, or the event sits in a
buffer and the client sees nothing until the buffer fills or the handler returns.
In Go you assert the `http.Flusher` interface:

```go
flusher, ok := w.(http.Flusher)
if !ok {
	http.Error(w, "streaming unsupported", http.StatusInternalServerError)
	return
}
// ... write an event ...
flusher.Flush() // push it to the client *now*
```

We also set `X-Accel-Buffering: no` to tell reverse proxies (nginx) not to buffer
the stream on their side. Buffering is the natural enemy of streaming: every
layer wants to batch bytes for efficiency, and every layer must be told not to.

---

## 1.4 The teaching bug: `ERR_INCOMPLETE_CHUNKED_ENCODING`

This project hit a real bug that is the perfect capstone for everything above.
The browser console showed, on all three SSE streams at once:

```
Failed to load resource: net::ERR_INCOMPLETE_CHUNKED_ENCODING
```

Now you have the vocabulary to understand it exactly. Recall:

- SSE responses use **chunked transfer encoding** (§1.2 Answer B).
- A chunked response is only *complete* when the server sends the **zero-length
  terminator chunk** `0\r\n\r\n`.
- `ERR_INCOMPLETE_CHUNKED_ENCODING` means precisely: *the connection closed while
  the response was still using chunked encoding and the terminator chunk was
  never received.* The browser was promised more chunks and the pipe died.

So: what was killing the stream without a terminator?

During development the server runs under a file-watcher (`air`) that **restarts
the process on every code change**. To restart, it sends the process `SIGINT`,
waits a short grace period, then `SIGKILL`s it. Our shutdown code looked correct:

```go
shutdownCtx, _ := context.WithTimeout(context.Background(), 30*time.Second)
server.Shutdown(shutdownCtx)
```

But here's the subtlety that ties the whole stack together. `server.Shutdown()`
**waits for in-flight requests to finish** — it does *not* forcibly cancel them.
Our SSE handlers are infinite loops that only return when their **request
context** is cancelled:

```go
for {
	select {
	case e := <-ch:
		writeSSE(w, "store-change", e)
		flusher.Flush()
	case <-r.Context().Done(): // when does THIS fire?
		return
	}
}
```

By default, `r.Context()` is *not* cancelled by `Shutdown()`. So on SIGINT:

1. `Shutdown()` politely waits for the SSE handlers to return.
2. The SSE handlers politely wait for `r.Context()` to be cancelled.
3. Nobody cancels it. `Shutdown()` blocks for its full 30-second timeout.
4. Meanwhile the watcher's grace period expires and it `SIGKILL`s the process.
5. `SIGKILL` is instant and uninterruptible. The kernel tears down the TCP
   sockets. The chunked streams die **mid-chunk, with no terminator**.
6. Three open SSE streams → three `ERR_INCOMPLETE_CHUNKED_ENCODING` errors.

The streams were never broken. The *shutdown* was breaking them, abruptly.

### The fix, and why it's exactly right

We wire every request's context to the signal context, using `BaseContext`:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
// ...
server := &http.Server{
	// ...
	BaseContext: func(net.Listener) context.Context { return ctx },
}
```

`BaseContext` makes the signal context the **parent** of every request context.
Now on SIGINT:

1. `ctx` is cancelled.
2. That cancellation propagates to every `r.Context()`.
3. Every SSE handler's `case <-r.Context().Done()` fires and the handler
   **returns normally**.
4. Because the handler returns normally, Go's HTTP stack **finishes the chunked
   response properly** — it writes the `0\r\n\r\n` terminator and closes the
   connection cleanly.
5. The browser sees a *complete* response. `EventSource` treats a clean close as
   "reconnect," not as an error. **The console stays silent.**
6. `server.Shutdown()` returns in milliseconds because all handlers exited.

Measured before and after: shutdown went from a 500 ms forced kill to a **19 ms
clean exit**, and a held connection closed with a proper terminator instead of an
abrupt reset.

**The moral.** The bug was invisible at the framework level — the SSE code was
fine, the shutdown call looked fine. It only made sense when you understood that
chunked encoding has a terminator, that a clean handler return writes that
terminator, and that a `SIGKILL` skips it. *Networking knowledge debugged a
problem that framework knowledge could not.* That is the entire thesis of these
docs in one story.

---

## 1.5 What to take away

- TCP is an ordered byte stream with **no message boundaries**. Everything above
  it has to invent its own framing.
- HTTP frames a response either by `Content-Length` (known size) or **chunked
  encoding** (streaming, unknown size, terminated by a zero chunk).
- **SSE is not a protocol** — it's chunked HTTP with a `text/event-stream`
  content type and a three-line text convention. You can write it with
  `fmt.Fprintf`.
- Streaming requires **flushing** at every layer and telling proxies **not to
  buffer**.
- "Real-time" on the web is mostly just *not closing the connection*. The
  exotic-sounding feature is built from the most ordinary parts.
- When something breaks "in the framework," drop a layer. The answer is often in
  the framing.

Next: [how Go turns these primitives into a clean, concurrent backend →](./02-go-backend.md)

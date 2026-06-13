package gohtmxelm

// ProtocolVersion is the broker envelope version. The browser broker and every
// Elm island stamp and validate this exact value; bumping it is a deliberate,
// breaking change to the wire contract.
const ProtocolVersion = 1

// Broker message types. These are the only types the generic broker
// understands. Application-specific events travel as the payload of a generic
// type (typically SSE_EVENT), so adding a new application event never requires
// touching the broker.
const (
	// TypeReady is sent by an island once its ports are wired; the broker
	// replies with TypeBrokerReady.
	TypeReady = "READY"
	// TypeBrokerReady acknowledges an island and completes the handshake.
	TypeBrokerReady = "BROKER_READY"
	// TypeStateSet sets one key in shared broker state and routes onward.
	TypeStateSet = "STATE_SET"
	// TypeStatePatch merges an object into shared broker state.
	TypeStatePatch = "STATE_PATCH"
	// TypeSend delivers an opaque payload to a target without touching state.
	TypeSend = "SEND"
	// TypeHTMXSwap asks the broker to run an htmx.ajax swap on a selector.
	TypeHTMXSwap = "HTMX_SWAP"
	// TypeSSEEvent carries one forwarded Server-Sent Event: payload is
	// {event, data}. This is how all server-pushed application state reaches
	// islands generically.
	TypeSSEEvent = "SSE_EVENT"
	// TypeHTMXAfterSwap is broadcast to islands after an htmx swap settles so
	// they can react to server-rendered fragment changes; payload is
	// {targetId, url}.
	TypeHTMXAfterSwap = "HTMX_AFTER_SWAP"
)

// Routing targets understood by the broker's router.
const (
	TargetBroker    = "broker"    // handled internally
	TargetBroadcast = "broadcast" // every island
	TargetOthers    = "others"    // every island except the sender
)

// Envelope is the single message shape exchanged between islands and the
// broker. It exists in Go so servers can construct and validate broker
// messages with the same contract the browser and Elm use, rather than
// duplicating string literals. source is stamped by the broker, never the
// sender.
type Envelope struct {
	Version int    `json:"version"`
	Type    string `json:"type"`
	Source  string `json:"source,omitempty"`
	Target  string `json:"target"`
	Payload any    `json:"payload,omitempty"`
}

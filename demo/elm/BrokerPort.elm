port module BrokerPort exposing
    ( Inbound(..)
    , Model
    , StoreChange
    , brokerIn
    , brokerOut
    , brokerState
    , decode
    , initialModel
    , ready
    , sendHtmxSwap
    , sendMessage
    , sendStateSet
    , storeChangeFromData
    )

import Dict exposing (Dict)
import Json.Decode as Decode
import Json.Encode as Encode


{-| The broker ports are declared ONCE here, in the shared port module, and
reused by every island. Declaring them per-island would register the same port
name multiple times in a single compiled bundle, which Elm rejects at load
("There can only be one port named brokerIn"). Each `Browser.element` instance
still gets its own isolated port wiring.
-}
port brokerOut : Encode.Value -> Cmd msg


port brokerIn : (Decode.Value -> msg) -> Sub msg


protocolVersion : Int
protocolVersion =
    1


type alias Model =
    { islandId : String
    , received : String
    , brokerReady : Bool
    , storeState : Dict String String
    , lastHtmxSwap : Maybe String
    , lastWrite : Maybe StoreChange
    }


initialModel : String -> Model
initialModel islandId =
    { islandId = islandId
    , received = ""
    , brokerReady = False
    , storeState = Dict.empty
    , lastHtmxSwap = Nothing
    , lastWrite = Nothing
    }


{-| One store mutation, with attribution: which surface (htmx, datastar, app-a,
app-b, go) performed it.
-}
type alias StoreChange =
    { key : String
    , value : String
    , source : String
    , deleted : Bool
    }


{-| Inbound is the typed classification of a broker envelope. Each island
pattern-matches the cases it cares about and ignores the rest, so the broker
can grow new event types without breaking existing islands.

  - `BrokerReady` — the handshake reply; the broker is wired to this island.
  - `Sse name data` — a forwarded Server-Sent Event with its raw JSON data.
  - `HtmxAfterSwap target` — an htmx fragment swap settled (id of the target).
  - `StateChanged` — shared broker state was set/patched; read it with `brokerState`.
  - `Other` — anything not classified above.

-}
type Inbound
    = BrokerReady
    | Sse String Decode.Value
    | HtmxAfterSwap (Maybe String)
    | StateChanged
    | Other


{-| Classify an inbound envelope. `brokerState` is always available regardless
of the case, so callers read shared state separately.
-}
decode : Decode.Value -> Inbound
decode value =
    case field "type" Decode.string value of
        Just "BROKER_READY" ->
            BrokerReady

        Just "SSE_EVENT" ->
            Sse
                (at [ "payload", "event" ] Decode.string value |> Maybe.withDefault "")
                (at [ "payload", "data" ] Decode.value value |> Maybe.withDefault Encode.null)

        Just "HTMX_AFTER_SWAP" ->
            HtmxAfterSwap (at [ "payload", "targetId" ] Decode.string value)

        Just "STATE_SET" ->
            StateChanged

        Just "STATE_PATCH" ->
            StateChanged

        _ ->
            Other


{-| The shared broker state dict, present on every inbound envelope. Non-string
values are dropped (the reference demo only stores strings).
-}
brokerState : Decode.Value -> Dict String String
brokerState value =
    at [ "brokerState" ] (Decode.dict Decode.string) value
        |> Maybe.withDefault Dict.empty


{-| Decode one store mutation from forwarded SSE data (a store-change or
store-hydrate payload).
-}
storeChangeFromData : Decode.Value -> Maybe StoreChange
storeChangeFromData data =
    case field "key" Decode.string data of
        Just key ->
            Just
                { key = key
                , value = field "value" Decode.string data |> Maybe.withDefault ""
                , source = field "source" Decode.string data |> Maybe.withDefault "unknown"
                , deleted = field "deleted" Decode.bool data |> Maybe.withDefault False
                }

        Nothing ->
            Nothing



-- OUTBOUND


ready : Cmd msg
ready =
    brokerOut (envelope "READY" "broker" (Encode.object []))


{-| Persist a key/value pair in shared broker state and route to target. The
host page mirrors the write to its server store.
-}
sendStateSet : String -> String -> Encode.Value -> Cmd msg
sendStateSet target key value =
    brokerOut
        (envelope "STATE_SET"
            target
            (Encode.object [ ( "key", Encode.string key ), ( "value", value ) ])
        )


{-| Send an ephemeral message to target without mutating broker state.
-}
sendMessage : String -> Encode.Value -> Cmd msg
sendMessage target payload =
    brokerOut (envelope "SEND" target payload)


{-| Ask the broker to run an htmx.ajax GET swap on a CSS selector, with no
Elm→server round-trip.
-}
sendHtmxSwap : String -> String -> Cmd msg
sendHtmxSwap selector url =
    brokerOut
        (envelope "HTMX_SWAP"
            "broker"
            (Encode.object [ ( "selector", Encode.string selector ), ( "url", Encode.string url ) ])
        )


envelope : String -> String -> Encode.Value -> Encode.Value
envelope type_ target payload =
    Encode.object
        [ ( "version", Encode.int protocolVersion )
        , ( "type", Encode.string type_ )
        , ( "target", Encode.string target )
        , ( "payload", payload )
        ]



-- DECODE HELPERS


field : String -> Decode.Decoder a -> Decode.Value -> Maybe a
field name decoder value =
    Decode.decodeValue (Decode.field name decoder) value
        |> Result.toMaybe


at : List String -> Decode.Decoder a -> Decode.Value -> Maybe a
at path decoder value =
    Decode.decodeValue (Decode.at path decoder) value
        |> Result.toMaybe

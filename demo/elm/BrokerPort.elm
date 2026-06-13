module BrokerPort exposing
    ( Inbound
    , Model
    , StoreChange
    , decodeInbound
    , initialModel
    , ready
    , sendHtmxSwap
    , sendMessage
    , sendStateSet
    )

import Dict exposing (Dict)
import Json.Decode as Decode
import Json.Encode as Encode


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
    , received = "Nothing yet"
    , brokerReady = False
    , storeState = Dict.empty
    , lastHtmxSwap = Nothing
    , lastWrite = Nothing
    }


{-| One store mutation observed via SSE, with attribution: which surface
(htmx, datastar, app-a, app-b, go) performed it.
-}
type alias StoreChange =
    { key : String
    , value : String
    , source : String
    , deleted : Bool
    }


type alias Inbound =
    { message : String
    , brokerReady : Bool
    , storeState : Dict String String
    , htmxSwapTarget : Maybe String
    , storeChange : Maybe StoreChange
    }


ready : (Encode.Value -> Cmd msg) -> Cmd msg
ready out =
    out
        (Encode.object
            [ ( "version", Encode.int 1 )
            , ( "type", Encode.string "READY" )
            , ( "target", Encode.string "broker" )
            , ( "payload", Encode.object [] )
            ]
        )


{-| Persist a key/value pair in shared broker state and route to target.
The change is also mirrored to the Go KV store by broker.js.
-}
sendStateSet : (Encode.Value -> Cmd msg) -> String -> String -> Encode.Value -> Cmd msg
sendStateSet out target key value =
    out
        (Encode.object
            [ ( "version", Encode.int 1 )
            , ( "type", Encode.string "STATE_SET" )
            , ( "target", Encode.string target )
            , ( "payload"
              , Encode.object
                    [ ( "key", Encode.string key )
                    , ( "value", value )
                    ]
              )
            ]
        )


{-| Send an ephemeral message to target without mutating broker state.
Use for one-shot events that should not replay to late-mounting islands.
-}
sendMessage : (Encode.Value -> Cmd msg) -> String -> Encode.Value -> Cmd msg
sendMessage out target payload =
    out
        (Encode.object
            [ ( "version", Encode.int 1 )
            , ( "type", Encode.string "SEND" )
            , ( "target", Encode.string target )
            , ( "payload", payload )
            ]
        )


{-| Gap 1: ask broker.js to perform an htmx.ajax GET swap on a DOM element,
without any Elm→server round-trip.
selector is a CSS selector (e.g. "#server-message").
url is the HTMX fragment endpoint (e.g. "/message").
-}
sendHtmxSwap : (Encode.Value -> Cmd msg) -> String -> String -> Cmd msg
sendHtmxSwap out selector url =
    out
        (Encode.object
            [ ( "version", Encode.int 1 )
            , ( "type", Encode.string "HTMX_SWAP" )
            , ( "target", Encode.string "broker" )
            , ( "payload"
              , Encode.object
                    [ ( "selector", Encode.string selector )
                    , ( "url", Encode.string url )
                    ]
              )
            ]
        )


decodeInbound : Decode.Value -> Result Decode.Error Inbound
decodeInbound =
    Decode.decodeValue
        (Decode.map5 Inbound
            -- message: read from shared broker state
            (Decode.oneOf
                [ Decode.field "brokerState" (Decode.field "message" Decode.string)
                , Decode.succeed "No message in broker state"
                ]
            )
            -- brokerReady: true only on the BROKER_READY handshake event
            (Decode.oneOf
                [ Decode.field "type" Decode.string
                    |> Decode.andThen (\t -> Decode.succeed (t == "BROKER_READY"))
                , Decode.succeed False
                ]
            )
            -- storeState: full broker state dict; falls back to empty if any
            -- value is non-string (shouldn't happen in this app)
            (Decode.oneOf
                [ Decode.field "brokerState" (Decode.dict Decode.string)
                , Decode.succeed Dict.empty
                ]
            )
            -- htmxSwapTarget: Gap 2 — present only on HTMX_AFTER_SWAP events
            (Decode.oneOf
                [ Decode.field "type" Decode.string
                    |> Decode.andThen
                        (\t ->
                            if t == "HTMX_AFTER_SWAP" then
                                Decode.oneOf
                                    [ Decode.at [ "payload", "targetId" ] (Decode.nullable Decode.string)
                                    , Decode.succeed Nothing
                                    ]

                            else
                                Decode.succeed Nothing
                        )
                , Decode.succeed Nothing
                ]
            )
            -- storeChange: present only on STORE_CHANGE events; carries
            -- attribution so islands know who wrote without trusting the DOM
            (Decode.oneOf
                [ Decode.field "type" Decode.string
                    |> Decode.andThen
                        (\t ->
                            if t == "STORE_CHANGE" then
                                Decode.map Just decodeStoreChange

                            else
                                Decode.succeed Nothing
                        )
                , Decode.succeed Nothing
                ]
            )
        )


decodeStoreChange : Decode.Decoder StoreChange
decodeStoreChange =
    Decode.map4 StoreChange
        (Decode.at [ "payload", "key" ] Decode.string)
        (Decode.oneOf
            [ Decode.at [ "payload", "value" ] Decode.string
            , Decode.succeed ""
            ]
        )
        (Decode.oneOf
            [ Decode.at [ "payload", "source" ] Decode.string
            , Decode.succeed "unknown"
            ]
        )
        (Decode.oneOf
            [ Decode.at [ "payload", "deleted" ] Decode.bool
            , Decode.succeed False
            ]
        )

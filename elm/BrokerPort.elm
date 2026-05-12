module BrokerPort exposing
    ( Inbound
    , Model
    , decodeInbound
    , initialModel
    , ready
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
    }


initialModel : String -> Model
initialModel islandId =
    { islandId = islandId
    , received = "Nothing yet"
    , brokerReady = False
    , storeState = Dict.empty
    }


type alias Inbound =
    { message : String
    , brokerReady : Bool
    , storeState : Dict String String
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


decodeInbound : Decode.Value -> Result Decode.Error Inbound
decodeInbound =
    Decode.decodeValue
        (Decode.map3 Inbound
            (Decode.oneOf
                [ Decode.field "brokerState" (Decode.field "message" Decode.string)
                , Decode.succeed "No message in broker state"
                ]
            )
            (Decode.oneOf
                [ Decode.field "type" Decode.string
                    |> Decode.andThen (\t -> Decode.succeed (t == "BROKER_READY"))
                , Decode.succeed False
                ]
            )
            -- brokerState is kept in sync with the Go KV store by broker.js;
            -- fall back to empty dict if the field is absent or contains non-string values.
            (Decode.oneOf
                [ Decode.field "brokerState" (Decode.dict Decode.string)
                , Decode.succeed Dict.empty
                ]
            )
        )

module BrokerPort exposing
    ( Inbound
    , Model
    , decodeInbound
    , initialModel
    , ready
    , sendMessage
    , sendStateSet
    )

import Json.Decode as Decode
import Json.Encode as Encode


type alias Model =
    { islandId : String
    , received : String
    , brokerReady : Bool
    }


initialModel : String -> Model
initialModel islandId =
    { islandId = islandId
    , received = "Nothing yet"
    , brokerReady = False
    }


type alias Inbound =
    { message : String
    , brokerReady : Bool
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


{-| Persist a key/value pair in broker shared state and route to target.
The value can be any JSON-encodable type.
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
Use this for one-shot events that should not be replayed to late-mounting islands.
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
        (Decode.map2 Inbound
            (Decode.oneOf
                [ Decode.field "brokerState" (Decode.field "message" Decode.string)
                , Decode.succeed "No message in broker state"
                ]
            )
            (Decode.oneOf
                [ Decode.field "type" Decode.string
                    |> Decode.andThen
                        (\t -> Decode.succeed (t == "BROKER_READY"))
                , Decode.succeed False
                ]
            )
        )

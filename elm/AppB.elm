port module AppB exposing (main)

import Browser
import Html exposing (Html, button, div, p, strong, text)
import Html.Events exposing (onClick)
import Json.Decode as Decode
import Json.Encode as Encode


port brokerOut : Encode.Value -> Cmd msg


port brokerIn : (Decode.Value -> msg) -> Sub msg


type alias Model =
    { islandId : String
    , received : String
    , brokerReady : Bool
    }


type Msg
    = SendToA
    | Broadcast
    | BrokerIn Decode.Value


main : Program Decode.Value Model Msg
main =
    Browser.element
        { init = init
        , update = update
        , view = view
        , subscriptions = \_ -> brokerIn BrokerIn
        }


init : Decode.Value -> ( Model, Cmd Msg )
init flags =
    let
        islandId =
            Decode.decodeValue (Decode.field "islandId" Decode.string) flags
                |> Result.withDefault "app-b"
    in
    ( { islandId = islandId
      , received = "Nothing yet"
      , brokerReady = False
      }
    , ready
    )


ready : Cmd Msg
ready =
    brokerOut
        (Encode.object
            [ ( "version", Encode.int 1 )
            , ( "type", Encode.string "READY" )
            , ( "target", Encode.string "broker" )
            , ( "payload", Encode.object [] )
            ]
        )


sendStateSet : String -> String -> String -> Cmd Msg
sendStateSet target key value =
    brokerOut
        (Encode.object
            [ ( "version", Encode.int 1 )
            , ( "type", Encode.string "STATE_SET" )
            , ( "target", Encode.string target )
            , ( "payload"
              , Encode.object
                    [ ( "key", Encode.string key )
                    , ( "value", Encode.string value )
                    ]
              )
            ]
        )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        SendToA ->
            ( model
            , sendStateSet "app-a" "message" "Hello from App B to App A"
            )

        Broadcast ->
            ( model
            , sendStateSet "broadcast" "message" "Hello from App B to everyone"
            )

        BrokerIn value ->
            case decodeInbound value of
                Ok inbound ->
                    ( { model
                        | received = inbound.message
                        , brokerReady = inbound.brokerReady || model.brokerReady
                      }
                    , Cmd.none
                    )

                Err _ ->
                    ( model, Cmd.none )


type alias Inbound =
    { message : String
    , brokerReady : Bool
    }


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
                        (\eventType ->
                            if eventType == "BROKER_READY" then
                                Decode.succeed True

                            else
                                Decode.succeed False
                        )
                , Decode.succeed False
                ]
            )
        )


view : Model -> Html Msg
view model =
    div []
        [ p []
            [ strong [] [ text "Island ID: " ]
            , text model.islandId
            ]
        , p []
            [ strong [] [ text "Broker ready: " ]
            , text
                (if model.brokerReady then
                    "yes"

                 else
                    "no"
                )
            ]
        , p []
            [ strong [] [ text "Received: " ]
            , text model.received
            ]
        , button [ onClick SendToA ] [ text "Send to App A" ]
        , text " "
        , button [ onClick Broadcast ] [ text "Broadcast" ]
        ]

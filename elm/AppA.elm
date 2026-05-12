port module AppA exposing (main)

import BrokerPort exposing (Inbound, Model, decodeInbound, initialModel, ready, sendStateSet)
import Browser
import Html exposing (Html, button, div, p, strong, text)
import Html.Events exposing (onClick)
import Json.Decode as Decode
import Json.Encode as Encode


port brokerOut : Encode.Value -> Cmd msg


port brokerIn : (Decode.Value -> msg) -> Sub msg


type Msg
    = SendToB
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
                |> Result.withDefault "app-a"
    in
    ( initialModel islandId, ready brokerOut )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        SendToB ->
            ( model
            , sendStateSet brokerOut "app-b" "message" (Encode.string "Hello from App A to App B")
            )

        Broadcast ->
            ( model
            , sendStateSet brokerOut "broadcast" "message" (Encode.string "Hello from App A to everyone")
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

                Err err ->
                    let
                        _ =
                            Debug.log "AppA BrokerIn decode error" (Decode.errorToString err)
                    in
                    ( model, Cmd.none )


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
        , button [ onClick SendToB ] [ text "Send to App B" ]
        , text " "
        , button [ onClick Broadcast ] [ text "Broadcast" ]
        ]

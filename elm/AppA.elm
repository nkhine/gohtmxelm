port module AppA exposing (main)

import BrokerPort exposing (Inbound, Model, decodeInbound, initialModel, ready, sendHtmxSwap, sendStateSet)
import Browser
import Dict
import Html exposing (Html, button, div, h3, p, strong, table, tbody, td, text, th, thead, tr)
import Html.Events exposing (onClick)
import Json.Decode as Decode
import Json.Encode as Encode


port brokerOut : Encode.Value -> Cmd msg


port brokerIn : (Decode.Value -> msg) -> Sub msg


type Msg
    = SendToB
    | Broadcast
    | RefreshServerMessage -- Gap 1: Elm triggers an HTMX swap
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

        RefreshServerMessage ->
            -- Gap 1: tells broker.js to call htmx.ajax — no Go round-trip from Elm.
            ( model
            , sendHtmxSwap brokerOut "#server-message" "/message"
            )

        BrokerIn value ->
            case decodeInbound value of
                Ok inbound ->
                    ( { model
                        | received = inbound.message
                        , brokerReady = inbound.brokerReady || model.brokerReady
                        , storeState = inbound.storeState
                        , lastHtmxSwap =
                            -- Gap 2: preserve the last swap target seen
                            case inbound.htmxSwapTarget of
                                Just _ ->
                                    inbound.htmxSwapTarget

                                Nothing ->
                                    model.lastHtmxSwap
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
        , text " "
        , button [ onClick RefreshServerMessage ] [ text "Refresh server message (Elm\u{2192}HTMX)" ]
        , viewLastSwap model.lastHtmxSwap
        , viewStoreState model.storeState
        ]


viewLastSwap : Maybe String -> Html Msg
viewLastSwap lastSwap =
    p []
        [ strong [] [ text "Last HTMX swap: " ]
        , text (Maybe.withDefault "none yet" lastSwap)
        ]


viewStoreState : Dict.Dict String String -> Html Msg
viewStoreState state =
    if Dict.isEmpty state then
        p [] [ text "Store: (empty)" ]

    else
        div []
            [ h3 [] [ text "Store state" ]
            , table []
                [ thead []
                    [ tr []
                        [ th [] [ text "Key" ]
                        , th [] [ text "Value" ]
                        ]
                    ]
                , tbody []
                    (List.map
                        (\( k, v ) ->
                            tr []
                                [ td [] [ text k ]
                                , td [] [ text v ]
                                ]
                        )
                        (Dict.toList state)
                    )
                ]
            ]

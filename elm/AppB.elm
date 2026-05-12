port module AppB exposing (main)

import BrokerPort exposing (Inbound, Model, decodeInbound, initialModel, ready, sendStateSet)
import Browser
import Dict
import Html exposing (Html, button, div, h3, p, span, strong, table, tbody, td, text, th, thead, tr)
import Html.Attributes exposing (class)
import Html.Events exposing (onClick)
import Json.Decode as Decode
import Json.Encode as Encode


port brokerOut : Encode.Value -> Cmd msg


port brokerIn : (Decode.Value -> msg) -> Sub msg


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
    ( initialModel islandId, ready brokerOut )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        SendToA ->
            ( model
            , sendStateSet brokerOut "app-a" "message" (Encode.string "Hello from App B to App A")
            )

        Broadcast ->
            ( model
            , sendStateSet brokerOut "broadcast" "message" (Encode.string "Hello from App B to everyone")
            )

        BrokerIn value ->
            case decodeInbound value of
                Ok inbound ->
                    ( { model
                        | received = inbound.message
                        , brokerReady = inbound.brokerReady || model.brokerReady
                        , storeState = inbound.storeState
                        , lastHtmxSwap =
                            -- Gap 2: passive observer — sees every swap that AppA (or HTMX) triggers
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
                            Debug.log "AppB BrokerIn decode error" (Decode.errorToString err)
                    in
                    ( model, Cmd.none )


view : Model -> Html Msg
view model =
    div []
        [ div [ class "field-row" ]
            [ span [ class "field-label" ] [ text "Island ID" ]
            , span [ class "field-value" ] [ text model.islandId ]
            ]
        , div [ class "field-row" ]
            [ span [ class "field-label" ] [ text "Broker" ]
            , if model.brokerReady then
                span [ class "badge-ready" ] [ text "ready" ]

              else
                span [ class "badge-waiting" ] [ text "waiting" ]
            ]
        , div [ class "field-row" ]
            [ span [ class "field-label" ] [ text "Received" ]
            , span [ class "field-value" ] [ text (nonempty model.received "(none)") ]
            ]
        , div [ class "field-row" ]
            [ span [ class "field-label" ] [ text "Last HTMX swap" ]
            , case model.lastHtmxSwap of
                Just target ->
                    span [ class "htmx-swap-tag" ] [ text target ]

                Nothing ->
                    span [ class "field-value" ] [ text "none yet" ]
            ]
        , div [ class "btn-group" ]
            [ button [ onClick SendToA ] [ text "Send to App A" ]
            , button [ onClick Broadcast ] [ text "Broadcast" ]
            ]
        , viewStoreState model.storeState
        ]


nonempty : String -> String -> String
nonempty s fallback =
    if String.isEmpty s then
        fallback

    else
        s


viewStoreState : Dict.Dict String String -> Html Msg
viewStoreState state =
    if Dict.isEmpty state then
        p [ class "field-row" ] [ text "Store: (empty)" ]

    else
        div []
            [ h3 [ class "field-label" ] [ text "Store snapshot" ]
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

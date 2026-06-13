port module AppB exposing (main)

import BrokerPort exposing (Model, StoreChange, decodeInbound, initialModel, ready, sendStateSet)
import Browser
import Html exposing (Html, button, div, h3, p, span, table, tbody, td, text, th, thead, tr)
import Html.Attributes exposing (class)
import Html.Events exposing (onClick)
import Json.Decode as Decode
import Json.Encode as Encode


port brokerOut : Encode.Value -> Cmd msg


port brokerIn : (Decode.Value -> msg) -> Sub msg


{-| App B is a typed event log: every store mutation that arrives over SSE
(no matter whether HTMX, Datastar, another Elm island, or Go wrote it) is
decoded into a StoreChange and folded into a bounded history list — modelling
an event stream is exactly what Elm's update loop is for.
-}
type alias AppModel =
    { shared : Model
    , history : List StoreChange
    }


historyLimit : Int
historyLimit =
    6


type Msg
    = SetSharedMessage
    | BrokerIn Decode.Value


main : Program Decode.Value AppModel Msg
main =
    Browser.element
        { init = init
        , update = update
        , view = view
        , subscriptions = \_ -> brokerIn BrokerIn
        }


init : Decode.Value -> ( AppModel, Cmd Msg )
init flags =
    let
        islandId =
            Decode.decodeValue (Decode.field "islandId" Decode.string) flags
                |> Result.withDefault "app-b"
    in
    ( { shared = initialModel islandId, history = [] }, ready brokerOut )


update : Msg -> AppModel -> ( AppModel, Cmd Msg )
update msg model =
    case msg of
        SetSharedMessage ->
            ( model
            , sendStateSet brokerOut "broadcast" "message" (Encode.string "Message written by Elm App B")
            )

        BrokerIn value ->
            case decodeInbound value of
                Ok inbound ->
                    let
                        shared =
                            model.shared
                    in
                    ( { model
                        | shared =
                            { shared
                                | received = inbound.message
                                , brokerReady = inbound.brokerReady || shared.brokerReady
                                , storeState = inbound.storeState
                            }
                        , history =
                            case inbound.storeChange of
                                Just change ->
                                    List.take historyLimit (change :: model.history)

                                Nothing ->
                                    model.history
                      }
                    , Cmd.none
                    )

                Err err ->
                    let
                        _ =
                            Debug.log "AppB BrokerIn decode error" (Decode.errorToString err)
                    in
                    ( model, Cmd.none )


view : AppModel -> Html Msg
view model =
    div []
        [ div [ class "field-row" ]
            [ span [ class "field-label" ] [ text "Broker" ]
            , if model.shared.brokerReady then
                span [ class "badge-ready" ] [ text "ready" ]

              else
                span [ class "badge-waiting" ] [ text "waiting" ]
            ]
        , div [ class "field-row" ]
            [ span [ class "field-label" ] [ text "Shared message" ]
            , span [ class "field-value" ] [ text (nonempty model.shared.received "(none)") ]
            ]
        , div [ class "btn-group" ]
            [ button [ onClick SetSharedMessage ] [ text "Save from Elm B" ] ]
        , viewHistory model.history
        ]


viewHistory : List StoreChange -> Html Msg
viewHistory history =
    if List.isEmpty history then
        p [ class "elm-hint" ] [ text "No store events seen yet — write from any pane." ]

    else
        div [ class "elm-history" ]
            [ h3 [ class "field-label" ]
                [ text ("Last " ++ String.fromInt historyLimit ++ " store events") ]
            , table []
                [ thead []
                    [ tr []
                        [ th [] [ text "By" ]
                        , th [] [ text "Key" ]
                        , th [] [ text "Value" ]
                        ]
                    ]
                , tbody [] (List.map viewChange history)
                ]
            ]


viewChange : StoreChange -> Html Msg
viewChange change =
    tr []
        [ td [] [ span [ class ("source-chip source-" ++ change.source) ] [ text change.source ] ]
        , td [] [ text change.key ]
        , td []
            [ if change.deleted then
                span [ class "elm-hint invalid" ] [ text "(deleted)" ]

              else
                text change.value
            ]
        ]


nonempty : String -> String -> String
nonempty s fallback =
    if String.isEmpty s then
        fallback

    else
        s

port module AppA exposing (main)

import BrokerPort exposing (Inbound(..), Model, brokerState, decode, initialModel, ready, sendHtmxSwap, sendStateSet, storeChangeFromData)
import Browser
import Dict
import Html exposing (Html, button, div, form, input, p, span, text)
import Html.Attributes exposing (class, disabled, placeholder, type_, value)
import Html.Events exposing (onClick, onInput, onSubmit)
import Json.Decode as Decode
import Json.Encode as Encode


port brokerOut : Encode.Value -> Cmd msg


port brokerIn : (Decode.Value -> msg) -> Sub msg


{-| App A is the "Elm strength" showcase: a draft editor whose validity is a
real state machine. The Save button cannot fire on an invalid draft because
the update function only emits a write in the Valid branch — the compiler
enforces it, not a runtime check.
-}
type alias AppModel =
    { shared : Model
    , draft : String
    }


maxDraftLength : Int
maxDraftLength =
    80


type Draft
    = Empty
    | TooLong Int
    | Valid String


classifyDraft : String -> Draft
classifyDraft raw =
    let
        trimmed =
            String.trim raw
    in
    if String.isEmpty trimmed then
        Empty

    else if String.length trimmed > maxDraftLength then
        TooLong (String.length trimmed)

    else
        Valid trimmed


type Msg
    = DraftChanged String
    | SubmitDraft
    | RefreshServerMessage -- Gap 1: Elm triggers an HTMX swap
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
                |> Result.withDefault "app-a"
    in
    ( { shared = initialModel islandId, draft = "" }, ready brokerOut )


update : Msg -> AppModel -> ( AppModel, Cmd Msg )
update msg model =
    case msg of
        DraftChanged raw ->
            ( { model | draft = raw }, Cmd.none )

        SubmitDraft ->
            case classifyDraft model.draft of
                Valid trimmed ->
                    ( { model | draft = "" }
                    , sendStateSet brokerOut "broadcast" "message" (Encode.string trimmed)
                    )

                _ ->
                    -- Unreachable through the UI (button is disabled), but the
                    -- state machine guarantees no invalid write regardless.
                    ( model, Cmd.none )

        RefreshServerMessage ->
            -- Gap 1: tells broker.js to call htmx.ajax — no Go round-trip from Elm.
            ( model
            , sendHtmxSwap brokerOut "#server-message" "/message"
            )

        BrokerIn value ->
            let
                shared =
                    model.shared

                state =
                    brokerState value

                base =
                    { shared
                        | storeState = state
                        , received = Dict.get "message" state |> Maybe.withDefault shared.received
                    }
            in
            case decode value of
                BrokerReady ->
                    ( { model | shared = { base | brokerReady = True } }, Cmd.none )

                Sse name data ->
                    if name == "store-change" || name == "store-hydrate" then
                        ( { model | shared = { base | lastWrite = keepLatest (storeChangeFromData data) shared.lastWrite } }, Cmd.none )

                    else
                        ( { model | shared = base }, Cmd.none )

                HtmxAfterSwap target ->
                    -- Gap 2: preserve the last swap target seen.
                    ( { model | shared = { base | lastHtmxSwap = keepLatest target shared.lastHtmxSwap } }, Cmd.none )

                _ ->
                    ( { model | shared = base }, Cmd.none )


{-| Prefer a new value when present, otherwise keep the previous one.
-}
keepLatest : Maybe a -> Maybe a -> Maybe a
keepLatest incoming previous =
    case incoming of
        Just _ ->
            incoming

        Nothing ->
            previous


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
        , div [ class "field-row" ]
            [ span [ class "field-label" ] [ text "Last write by" ]
            , case model.shared.lastWrite of
                Just change ->
                    span [ class ("source-chip source-" ++ change.source) ]
                        [ text
                            (if change.deleted then
                                change.source ++ " (deleted " ++ change.key ++ ")"

                             else
                                change.source
                            )
                        ]

                Nothing ->
                    span [ class "field-value" ] [ text "no writes seen yet" ]
            ]
        , div [ class "field-row" ]
            [ span [ class "field-label" ] [ text "Last HTMX swap" ]
            , case model.shared.lastHtmxSwap of
                Just target ->
                    span [ class "htmx-swap-tag" ] [ text target ]

                Nothing ->
                    span [ class "field-value" ] [ text "none yet" ]
            ]
        , viewDraftEditor model.draft
        , div [ class "btn-group" ]
            [ button [ onClick RefreshServerMessage, class "btn-htmx-trigger" ]
                [ text "Refresh via HTMX" ]
            ]
        ]


viewDraftEditor : String -> Html Msg
viewDraftEditor draft =
    let
        state =
            classifyDraft draft

        ( hint, hintClass, savable ) =
            case state of
                Empty ->
                    ( "Type a message to enable Save — empty drafts are unrepresentable as writes."
                    , "elm-hint"
                    , False
                    )

                TooLong n ->
                    ( "Too long: " ++ String.fromInt n ++ "/" ++ String.fromInt maxDraftLength ++ " characters."
                    , "elm-hint invalid"
                    , False
                    )

                Valid trimmed ->
                    ( String.fromInt (String.length trimmed)
                        ++ "/"
                        ++ String.fromInt maxDraftLength
                        ++ " — will save key \"message\"."
                    , "elm-hint"
                    , True
                    )
    in
    div []
        [ form [ class "elm-form", onSubmit SubmitDraft ]
            [ input
                [ type_ "text"
                , value draft
                , placeholder "Typed draft from Elm A"
                , onInput DraftChanged
                ]
                []
            , button [ type_ "submit", disabled (not savable) ] [ text "Save from Elm A" ]
            ]
        , p [ class hintClass ] [ text hint ]
        ]


nonempty : String -> String -> String
nonempty s fallback =
    if String.isEmpty s then
        fallback

    else
        s

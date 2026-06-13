module RangePicker exposing (main)

import BrokerPort exposing (Inbound(..), brokerIn, decode, ready, sendStateSet)
import Browser
import Html exposing (Html, button, div, input, p, span, text)
import Html.Attributes exposing (class, disabled, type_, value)
import Html.Events exposing (onClick, onInput)
import Json.Decode as Decode
import Json.Encode as Encode


{-| RangePicker is the Elm member of the bank-statement fusion. It is a typed
state machine: it owns the date-range selection (a preset or a validated custom
window) and never trusts itself with the data — it sends the *intent* to Go,
which resolves it against the server clock, filters the seeded transfers, and
fans the result back out over SSE. The picker then reflects the server-confirmed
active range. A custom range that fails validation simply cannot be applied —
the Apply button is disabled and `update` emits no write.
-}
type alias Model =
    { brokerReady : Bool
    , activePreset : Maybe String
    , fromStr : String
    , toStr : String
    , activeLabel : String
    , activeCount : Maybe Int
    }


type alias Preset =
    { key : String, label : String }


presets : List Preset
presets =
    [ Preset "15m" "15 min"
    , Preset "30m" "30 min"
    , Preset "1h" "1 hour"
    , Preset "3h" "3 hours"
    , Preset "24h" "24 hours"
    , Preset "2d" "2 days"
    , Preset "2w" "2 weeks"
    , Preset "1mo" "1 month"
    , Preset "3mo" "3 months"
    ]


type Msg
    = PickPreset String
    | FromChanged String
    | ToChanged String
    | ApplyCustom
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
init _ =
    ( { brokerReady = False
      , activePreset = Just "24h"
      , fromStr = ""
      , toStr = ""
      , activeLabel = "Last 24 hours"
      , activeCount = Nothing
      }
    , ready
    )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        PickPreset key ->
            ( { model | activePreset = Just key }
            , sendRange (presetPayload key)
            )

        FromChanged s ->
            ( { model | fromStr = s }, Cmd.none )

        ToChanged s ->
            ( { model | toStr = s }, Cmd.none )

        ApplyCustom ->
            case validateCustom model.fromStr model.toStr of
                Just ( from, to ) ->
                    ( { model | activePreset = Nothing }
                    , sendRange (customPayload from to)
                    )

                Nothing ->
                    -- Unreachable through the UI (Apply is disabled), but the
                    -- state machine guarantees no invalid range is ever sent.
                    ( model, Cmd.none )

        BrokerIn raw ->
            case decode raw of
                BrokerReady ->
                    ( { model | brokerReady = True }, Cmd.none )

                Sse "statement-range-change" data ->
                    ( { model
                        | activeLabel = stringField "label" model.activeLabel data
                        , activeCount = intField "count" data
                      }
                    , Cmd.none
                    )

                _ ->
                    ( model, Cmd.none )


{-| A custom range is valid when both ends parse as datetime-local strings and
"from" is not after "to". datetime-local is ISO-ordered, so a lexical compare is
also a chronological compare.
-}
validateCustom : String -> String -> Maybe ( String, String )
validateCustom from to =
    if String.isEmpty from || String.isEmpty to then
        Nothing

    else if from <= to then
        Just ( from, to )

    else
        Nothing


sendRange : String -> Cmd Msg
sendRange payload =
    -- The value is a JSON *string* so shared broker state stays string-typed
    -- (other islands read it as a string dict). The host (demo-ui.js) parses it
    -- and POSTs the range to Go.
    sendStateSet "broker" "statementRange" (Encode.string payload)


presetPayload : String -> String
presetPayload key =
    Encode.encode 0 (Encode.object [ ( "preset", Encode.string key ) ])


customPayload : String -> String -> String
customPayload from to =
    Encode.encode 0
        (Encode.object
            [ ( "from", Encode.string from )
            , ( "to", Encode.string to )
            ]
        )


stringField : String -> String -> Decode.Value -> String
stringField name fallback data =
    Decode.decodeValue (Decode.field name Decode.string) data
        |> Result.withDefault fallback


intField : String -> Decode.Value -> Maybe Int
intField name data =
    Decode.decodeValue (Decode.field name Decode.int) data
        |> Result.toMaybe


view : Model -> Html Msg
view model =
    div []
        [ div [ class "field-row" ]
            [ span [ class "field-label" ] [ text "Broker" ]
            , if model.brokerReady then
                span [ class "badge-ready" ] [ text "ready" ]

              else
                span [ class "badge-waiting" ] [ text "waiting" ]
            ]
        , div [ class "range-presets" ] (List.map (presetButton model.activePreset) presets)
        , viewCustom model
        , viewActive model
        ]


presetButton : Maybe String -> Preset -> Html Msg
presetButton active preset =
    button
        [ class
            (if active == Just preset.key then
                "range-preset active"

             else
                "range-preset"
            )
        , onClick (PickPreset preset.key)
        ]
        [ text preset.label ]


viewCustom : Model -> Html Msg
viewCustom model =
    let
        valid =
            validateCustom model.fromStr model.toStr /= Nothing
    in
    div [ class "range-custom" ]
        [ span [ class "field-label" ] [ text "Custom" ]
        , input [ type_ "datetime-local", value model.fromStr, onInput FromChanged ] []
        , span [ class "range-arrow" ] [ text "→" ]
        , input [ type_ "datetime-local", value model.toStr, onInput ToChanged ] []
        , button [ onClick ApplyCustom, disabled (not valid) ] [ text "Apply" ]
        ]


viewActive : Model -> Html Msg
viewActive model =
    div [ class "range-active" ]
        [ span [ class "field-label" ] [ text "Showing" ]
        , span [ class "range-active-label" ] [ text model.activeLabel ]
        , case model.activeCount of
            Just n ->
                span [ class "range-active-count" ]
                    [ text (String.fromInt n ++ " transfers") ]

            Nothing ->
                span [ class "elm-hint" ] [ text "waiting for server…" ]
        ]

module RangePicker exposing (main)

import BrokerPort exposing (Inbound(..), brokerIn, decode, ready, sendStateSet)
import Browser
import Html exposing (Html, button, div, span, text)
import Html.Attributes exposing (class, classList)
import Html.Events exposing (onClick, onMouseEnter)
import Json.Decode as Decode
import Json.Encode as Encode


{-| RangePicker is the Elm member of the bank-statement fusion: a typed range
selector modelled on the starbase / CloudWatch pickers. Relative presets live on
the right; an absolute range is chosen on a two-month calendar by clicking a
start day then an end day, with a live hover preview and start/in-range/end
highlighting. It owns no data — it sends the chosen range to Go, which resolves
and broadcasts it; the picker then reflects the server-confirmed window.
-}
type alias Date =
    { year : Int, month : Int, day : Int }


type alias Model =
    { brokerReady : Bool
    , activePreset : Maybe String
    , today : Maybe Date
    , month : Maybe Date -- first day of the left-hand visible month
    , selStart : Maybe Date -- armed start day during selection
    , selHover : Maybe Date
    , committed : Maybe ( Date, Date ) -- last committed custom range
    , activeLabel : String
    , activeCount : Maybe Int
    }


type alias Preset =
    { key : String, label : String }


presets : List Preset
presets =
    [ Preset "15m" "Last 15 min"
    , Preset "30m" "Last 30 min"
    , Preset "1h" "Last 1 hour"
    , Preset "3h" "Last 3 hours"
    , Preset "24h" "Last 24 hours"
    , Preset "2d" "Last 2 days"
    , Preset "2w" "Last 2 weeks"
    , Preset "1mo" "Last 1 month"
    , Preset "3mo" "Last 3 months"
    ]


type Msg
    = PickPreset String
    | DayClick Date
    | DayHover Date
    | ShiftMonth Int
    | ClearSelection
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
      , today = Nothing
      , month = Nothing
      , selStart = Nothing
      , selHover = Nothing
      , committed = Nothing
      , activeLabel = "Last 24 hours"
      , activeCount = Nothing
      }
    , ready
    )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        PickPreset key ->
            ( { model | activePreset = Just key, committed = Nothing, selStart = Nothing, selHover = Nothing }
            , sendRange (presetPayload key)
            )

        DayHover d ->
            case model.selStart of
                Just _ ->
                    ( { model | selHover = Just d }, Cmd.none )

                Nothing ->
                    ( model, Cmd.none )

        DayClick d ->
            case model.selStart of
                Nothing ->
                    -- First click arms the range; second click commits it.
                    ( { model | selStart = Just d, selHover = Just d }, Cmd.none )

                Just first ->
                    let
                        ( lo, hi ) =
                            order first d
                    in
                    ( { model
                        | selStart = Nothing
                        , selHover = Nothing
                        , committed = Just ( lo, hi )
                        , activePreset = Nothing
                      }
                    , sendRange (customPayload lo hi)
                    )

        ShiftMonth n ->
            ( { model | month = Maybe.map (shiftMonth n) model.month }, Cmd.none )

        ClearSelection ->
            ( { model | selStart = Nothing, selHover = Nothing }, Cmd.none )

        BrokerIn raw ->
            case decode raw of
                BrokerReady ->
                    ( { model | brokerReady = True }, Cmd.none )

                Sse "statement-range-change" data ->
                    let
                        td =
                            stringField "todayIso" "" data |> parseIso
                    in
                    ( { model
                        | activeLabel = stringField "label" model.activeLabel data
                        , activeCount = intField "count" data
                        , today = orElse td model.today
                        , month =
                            case ( model.month, td ) of
                                ( Nothing, Just t ) ->
                                    Just { t | day = 1 }

                                _ ->
                                    model.month
                      }
                    , Cmd.none
                    )

                _ ->
                    ( model, Cmd.none )



-- OUTBOUND


sendRange : String -> Cmd Msg
sendRange payload =
    sendStateSet "broker" "statementRange" (Encode.string payload)


presetPayload : String -> String
presetPayload key =
    Encode.encode 0 (Encode.object [ ( "preset", Encode.string key ) ])


{-| A calendar range covers whole days: start at 00:00, end at 23:59 (inclusive),
serialised as the datetime-local strings Go parses.
-}
customPayload : Date -> Date -> String
customPayload lo hi =
    Encode.encode 0
        (Encode.object
            [ ( "from", Encode.string (isoDateTime lo "00:00") )
            , ( "to", Encode.string (isoDateTime hi "23:59") )
            ]
        )



-- VIEW


view : Model -> Html Msg
view model =
    div [ class "range-picker" ]
        [ div [ class "range-head" ]
            [ span [ class "range-current" ] [ text (triggerLabel model) ]
            , case model.activeCount of
                Just n ->
                    span [ class "range-current-count" ] [ text (String.fromInt n ++ " transfers") ]

                Nothing ->
                    span [ class "elm-hint" ] [ text "waiting for server…" ]
            , if model.brokerReady then
                span [ class "badge-ready" ] [ text "ready" ]

              else
                span [ class "badge-waiting" ] [ text "waiting" ]
            ]
        , div [ class "range-body" ]
            [ div [ class "range-cals" ] (viewCalendars model)
            , div [ class "range-presets-col" ]
                (List.map (presetBtn model.activePreset) presets)
            ]
        , viewFooter model
        ]


viewCalendars : Model -> List (Html Msg)
viewCalendars model =
    case model.month of
        Just m ->
            [ calMonth model m True
            , calMonth model (shiftMonth 1 m) False
            ]

        Nothing ->
            [ div [ class "cal-loading" ] [ text "Loading calendar…" ] ]


calMonth : Model -> Date -> Bool -> Html Msg
calMonth model monthDate isLeft =
    div [ class "cal" ]
        [ div [ class "cal-header" ]
            [ if isLeft then
                button [ class "cal-nav", onClick (ShiftMonth -1) ] [ text "‹" ]

              else
                span [ class "cal-nav-spacer" ] []
            , span [ class "cal-month" ] [ text (monthName monthDate.month ++ " " ++ String.fromInt monthDate.year) ]
            , if isLeft then
                span [ class "cal-nav-spacer" ] []

              else
                button [ class "cal-nav", onClick (ShiftMonth 1) ] [ text "›" ]
            ]
        , div [ class "cal-weekdays" ]
            (List.map (\w -> span [] [ text w ]) [ "Mo", "Tu", "We", "Th", "Fr", "Sa", "Su" ])
        , div [ class "cal-grid" ]
            (List.repeat (weekdayMon { monthDate | day = 1 }) (span [ class "cal-blank" ] [])
                ++ List.map (calCell model monthDate) (List.range 1 (daysInMonth monthDate.year monthDate.month))
            )
        ]


calCell : Model -> Date -> Int -> Html Msg
calCell model monthDate dayNum =
    let
        d =
            { monthDate | day = dayNum }

        ( isIn, isStart, isEnd ) =
            case previewRange model of
                Just ( lo, hi ) ->
                    ( inRange lo hi d, sameDay d lo, sameDay d hi )

                Nothing ->
                    ( False, False, False )

        isToday =
            model.today == Just d
    in
    button
        [ classList
            [ ( "cal-day", True )
            , ( "in-range", isIn )
            , ( "range-start", isStart )
            , ( "range-end", isEnd )
            , ( "is-today", isToday )
            ]
        , onClick (DayClick d)
        , onMouseEnter (DayHover d)
        ]
        [ text (String.fromInt dayNum) ]


presetBtn : Maybe String -> Preset -> Html Msg
presetBtn active preset =
    button
        [ classList [ ( "range-preset", True ), ( "active", active == Just preset.key ) ]
        , onClick (PickPreset preset.key)
        ]
        [ text preset.label ]


viewFooter : Model -> Html Msg
viewFooter model =
    div [ class "range-footer" ]
        [ case ( model.selStart, previewRange model ) of
            ( Just _, Just ( lo, hi ) ) ->
                span [ class "range-hint" ]
                    [ text ("Start " ++ formatDate lo ++ " — click an end date (" ++ formatDate hi ++ ")") ]

            _ ->
                span [ class "range-hint" ] [ text "Click a start date, then an end date." ]
        , case model.selStart of
            Just _ ->
                button [ class "range-clear", onClick ClearSelection ] [ text "Cancel" ]

            Nothing ->
                text ""
        ]


{-| The range shown on the calendar: the live selection while arming, otherwise
the committed custom range.
-}
previewRange : Model -> Maybe ( Date, Date )
previewRange model =
    case ( model.selStart, model.selHover ) of
        ( Just s, Just h ) ->
            Just (order s h)

        ( Just s, Nothing ) ->
            Just ( s, s )

        ( Nothing, _ ) ->
            model.committed


triggerLabel : Model -> String
triggerLabel model =
    case model.activePreset of
        Just key ->
            presets |> List.filter (\p -> p.key == key) |> List.head |> Maybe.map .label |> Maybe.withDefault model.activeLabel

        Nothing ->
            case model.committed of
                Just ( lo, hi ) ->
                    formatDate lo ++ " – " ++ formatDate hi

                Nothing ->
                    model.activeLabel



-- DATE HELPERS


toInt : Date -> Int
toInt d =
    d.year * 10000 + d.month * 100 + d.day


sameDay : Date -> Date -> Bool
sameDay a b =
    toInt a == toInt b


inRange : Date -> Date -> Date -> Bool
inRange lo hi d =
    toInt d >= toInt lo && toInt d <= toInt hi


order : Date -> Date -> ( Date, Date )
order a b =
    if toInt a <= toInt b then
        ( a, b )

    else
        ( b, a )


isLeap : Int -> Bool
isLeap y =
    (modBy 4 y == 0 && modBy 100 y /= 0) || modBy 400 y == 0


daysInMonth : Int -> Int -> Int
daysInMonth y m =
    case m of
        2 ->
            if isLeap y then
                29

            else
                28

        4 ->
            30

        6 ->
            30

        9 ->
            30

        11 ->
            30

        _ ->
            31


{-| Day of week with Monday = 0 … Sunday = 6, via Sakamoto's algorithm.
-}
weekdayMon : Date -> Int
weekdayMon d =
    let
        t =
            [ 0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4 ]

        y =
            if d.month < 3 then
                d.year - 1

            else
                d.year

        tm =
            List.drop (d.month - 1) t |> List.head |> Maybe.withDefault 0

        sun0 =
            modBy 7 (y + (y // 4) - (y // 100) + (y // 400) + tm + d.day)
    in
    modBy 7 (sun0 + 6)


shiftMonth : Int -> Date -> Date
shiftMonth delta d =
    let
        total =
            (d.year * 12 + (d.month - 1)) + delta
    in
    { year = total // 12, month = modBy 12 total + 1, day = 1 }


monthName : Int -> String
monthName m =
    [ "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec" ]
        |> List.drop (m - 1)
        |> List.head
        |> Maybe.withDefault "?"


formatDate : Date -> String
formatDate d =
    String.fromInt d.day ++ " " ++ monthName d.month ++ " " ++ String.fromInt d.year


pad2 : Int -> String
pad2 n =
    String.padLeft 2 '0' (String.fromInt n)


isoDateTime : Date -> String -> String
isoDateTime d hm =
    String.fromInt d.year ++ "-" ++ pad2 d.month ++ "-" ++ pad2 d.day ++ "T" ++ hm


parseIso : String -> Maybe Date
parseIso s =
    case String.split "-" s |> List.map String.toInt of
        [ Just y, Just m, Just day ] ->
            Just { year = y, month = m, day = day }

        _ ->
            Nothing


orElse : Maybe a -> Maybe a -> Maybe a
orElse incoming previous =
    case incoming of
        Just _ ->
            incoming

        Nothing ->
            previous



-- DECODE HELPERS


stringField : String -> String -> Decode.Value -> String
stringField name fallback data =
    Decode.decodeValue (Decode.field name Decode.string) data
        |> Result.withDefault fallback


intField : String -> Decode.Value -> Maybe Int
intField name data =
    Decode.decodeValue (Decode.field name Decode.int) data
        |> Result.toMaybe

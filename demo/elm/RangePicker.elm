module RangePicker exposing (main)

import BrokerPort exposing (Inbound(..), brokerIn, decode, ready, sendStateSet)
import Browser
import Html exposing (Html, button, div, span, text)
import Html.Attributes exposing (class, classList)
import Html.Events exposing (onClick, onMouseEnter)
import Json.Decode as Decode
import Json.Encode as Encode


{-| RangePicker is the Elm member of the bank-statement fusion: a compact
CloudWatch-style range bar. Quick relative presets sit on a single bar; a
"Custom" button reveals a panel with two tabs — Absolute (a two-month calendar
range, click start then end) and Relative (pick "last N" by minutes / hours /
days / weeks). It owns no data: it sends the chosen range to Go, which resolves
and broadcasts it, then reflects the server-confirmed window.
-}
type alias Date =
    { year : Int, month : Int, day : Int }


type Tab
    = Absolute
    | Relative


type alias Model =
    { activeRel : Maybe ( Int, String ) -- (value, unit) of the active relative range
    , customOpen : Bool
    , tab : Tab
    , today : Maybe Date
    , month : Maybe Date
    , selStart : Maybe Date
    , selHover : Maybe Date
    , committed : Maybe ( Date, Date ) -- last committed absolute range
    , activeLabel : String
    , activeCount : Maybe Int
    }


{-| Quick-bar presets: (label, value, unit).
-}
quickPresets : List ( String, Int, String )
quickPresets =
    [ ( "1h", 1, "hours" )
    , ( "3h", 3, "hours" )
    , ( "12h", 12, "hours" )
    , ( "1d", 1, "days" )
    , ( "1w", 1, "weeks" )
    ]


{-| Relative-tab grids: (heading, unit, values).
-}
relGroups : List ( String, String, List Int )
relGroups =
    [ ( "Minutes", "minutes", [ 5, 10, 15, 20, 25, 30, 35, 40, 45 ] )
    , ( "Hours", "hours", List.range 1 12 )
    , ( "Days", "days", List.range 1 6 )
    , ( "Weeks", "weeks", List.range 1 4 )
    ]


type Msg
    = PickRelative Int String
    | ToggleCustom
    | SetTab Tab
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
    ( { activeRel = Just ( 1, "days" )
      , customOpen = False
      , tab = Absolute
      , today = Nothing
      , month = Nothing
      , selStart = Nothing
      , selHover = Nothing
      , committed = Nothing
      , activeLabel = "Last 1 day"
      , activeCount = Nothing
      }
    , ready
    )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        PickRelative value unit ->
            ( { model
                | activeRel = Just ( value, unit )
                , committed = Nothing
                , selStart = Nothing
                , selHover = Nothing
                , customOpen = False
              }
            , sendRange (relativePayload value unit)
            )

        ToggleCustom ->
            ( { model | customOpen = not model.customOpen }, Cmd.none )

        SetTab tab ->
            ( { model | tab = tab }, Cmd.none )

        DayHover d ->
            case model.selStart of
                Just _ ->
                    ( { model | selHover = Just d }, Cmd.none )

                Nothing ->
                    ( model, Cmd.none )

        DayClick d ->
            case model.selStart of
                Nothing ->
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
                        , activeRel = Nothing
                        , customOpen = False
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
                    ( model, Cmd.none )

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


relativePayload : Int -> String -> String
relativePayload value unit =
    Encode.encode 0
        (Encode.object [ ( "relValue", Encode.int value ), ( "relUnit", Encode.string unit ) ])


{-| A calendar range covers whole days: start at 00:00, end at 23:59 inclusive.
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
        [ div [ class "range-bar" ]
            (List.map (quickBtn model.activeRel) quickPresets
                ++ [ button
                        [ classList [ ( "range-custom-btn", True ), ( "active", model.customOpen || model.committed /= Nothing ) ]
                        , onClick ToggleCustom
                        ]
                        [ text "Custom ▾" ]
                   ]
            )
        , if model.customOpen then
            viewCustomPanel model

          else
            text ""
        ]


quickBtn : Maybe ( Int, String ) -> ( String, Int, String ) -> Html Msg
quickBtn activeRel ( label, value, unit ) =
    button
        [ classList [ ( "range-quick", True ), ( "active", activeRel == Just ( value, unit ) ) ]
        , onClick (PickRelative value unit)
        ]
        [ text label ]


viewCustomPanel : Model -> Html Msg
viewCustomPanel model =
    div [ class "range-custom-panel" ]
        [ div [ class "range-tabs" ]
            [ tabBtn model.tab Absolute "Absolute"
            , tabBtn model.tab Relative "Relative"
            ]
        , case model.tab of
            Absolute ->
                div [ class "range-abs" ]
                    [ div [ class "range-cals" ] (viewCalendars model)
                    , viewFooter model
                    ]

            Relative ->
                div [ class "range-rel" ] (List.map (relGroupView model.activeRel) relGroups)
        ]


tabBtn : Tab -> Tab -> String -> Html Msg
tabBtn current tab label =
    button
        [ classList [ ( "range-tab", True ), ( "active", current == tab ) ]
        , onClick (SetTab tab)
        ]
        [ text label ]


relGroupView : Maybe ( Int, String ) -> ( String, String, List Int ) -> Html Msg
relGroupView activeRel ( heading, unit, values ) =
    div [ class "rel-group" ]
        [ span [ class "rel-label" ] [ text heading ]
        , div [ class "rel-btns" ] (List.map (relBtn activeRel unit) values)
        ]


relBtn : Maybe ( Int, String ) -> String -> Int -> Html Msg
relBtn activeRel unit value =
    button
        [ classList [ ( "rel-btn", True ), ( "active", activeRel == Just ( value, unit ) ) ]
        , onClick (PickRelative value unit)
        ]
        [ text (String.fromInt value) ]


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
    in
    button
        [ classList
            [ ( "cal-day", True )
            , ( "in-range", isIn )
            , ( "range-start", isStart )
            , ( "range-end", isEnd )
            , ( "is-today", model.today == Just d )
            ]
        , onClick (DayClick d)
        , onMouseEnter (DayHover d)
        ]
        [ text (String.fromInt dayNum) ]


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


previewRange : Model -> Maybe ( Date, Date )
previewRange model =
    case ( model.selStart, model.selHover ) of
        ( Just s, Just h ) ->
            Just (order s h)

        ( Just s, Nothing ) ->
            Just ( s, s )

        ( Nothing, _ ) ->
            model.committed



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

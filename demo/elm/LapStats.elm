port module LapStats exposing (main)

import Browser
import BrokerPort exposing (Inbound(..), decode, ready)
import Html exposing (Html, div, span, text)
import Html.Attributes exposing (class)
import Json.Decode as Decode
import Json.Encode as Encode


port brokerOut : Encode.Value -> Cmd msg


port brokerIn : (Decode.Value -> msg) -> Sub msg


{-| LapStats is the Elm member of the stopwatch fusion. It owns no DOM the
other libraries touch: it subscribes to the same stopwatch SSE state (relayed
by broker.js as STOPWATCH_SNAPSHOT events) and derives typed lap analytics.
Deriving aggregates from an event stream with no nullable holes is exactly
what Elm's types are good at.
-}
type alias Lap =
    { number : Int
    , elapsedMs : Int
    }


type alias Model =
    { brokerReady : Bool
    , running : Bool
    , laps : List Lap
    }


{-| Analytics are only meaningful with at least one lap, so the empty case is
modelled explicitly rather than with sentinel zeros.
-}
type Stats
    = NoLaps
    | Stats
        { count : Int
        , fastest : Int
        , slowest : Int
        , average : Int
        , lastDelta : Maybe Int
        }


type Msg
    = BrokerIn Decode.Value


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
    ( { brokerReady = False, running = False, laps = [] }, ready brokerOut )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        BrokerIn value ->
            case decode value of
                BrokerReady ->
                    ( { model | brokerReady = True }, Cmd.none )

                Sse "stopwatch-state" data ->
                    ( { model
                        | running = decodeField [ "running" ] Decode.bool False data
                        , laps = decodeField [ "laps" ] (Decode.list decodeLap) [] data
                      }
                    , Cmd.none
                    )

                _ ->
                    ( model, Cmd.none )


decodeField : List String -> Decode.Decoder a -> a -> Decode.Value -> a
decodeField path decoder fallback value =
    Decode.decodeValue (Decode.at path decoder) value
        |> Result.withDefault fallback


decodeLap : Decode.Decoder Lap
decodeLap =
    Decode.map2 Lap
        (Decode.field "number" Decode.int)
        (Decode.field "elapsedMs" Decode.int)


{-| Laps arrive newest-first with cumulative elapsed times. Per-lap splits are
the differences between consecutive cumulative readings (oldest lap's split is
itself). This computes splits and their aggregates in one pass.
-}
computeStats : List Lap -> Stats
computeStats laps =
    case laps of
        [] ->
            NoLaps

        _ ->
            let
                -- Oldest-first cumulative times.
                cumulative =
                    List.reverse laps
                        |> List.map .elapsedMs

                splits =
                    diffs cumulative

                -- The most recent split is meaningful as a "delta from the
                -- previous lap" only once there are at least two laps.
                lastDelta =
                    if List.length splits >= 2 then
                        List.head (List.reverse splits)

                    else
                        Nothing
            in
            case splits of
                [] ->
                    NoLaps

                first :: rest ->
                    let
                        fastest =
                            List.foldl min first rest

                        slowest =
                            List.foldl max first rest

                        total =
                            List.sum splits

                        count =
                            List.length splits
                    in
                    Stats
                        { count = count
                        , fastest = fastest
                        , slowest = slowest
                        , average = total // count
                        , lastDelta = lastDelta
                        }


{-| Successive differences of a cumulative oldest-first series. The first
element passes through unchanged because it is measured from zero, e.g.
[c1, c2, c3] -> [c1, c2-c1, c3-c2].
-}
diffs : List Int -> List Int
diffs xs =
    case xs of
        [] ->
            []

        first :: rest ->
            first :: List.map2 (\prev cur -> cur - prev) xs rest


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
        , case computeStats model.laps of
            NoLaps ->
                div [ class "elm-hint" ]
                    [ text "No laps recorded. Start the stopwatch and hit Lap to feed typed analytics here." ]

            Stats s ->
                div [ class "lap-stats-grid" ]
                    [ stat "Laps" (String.fromInt s.count)
                    , stat "Fastest" (formatMs s.fastest)
                    , stat "Slowest" (formatMs s.slowest)
                    , stat "Average" (formatMs s.average)
                    , stat "Last split"
                        (case s.lastDelta of
                            Just d ->
                                formatMs d

                            Nothing ->
                                "—"
                        )
                    ]
        ]


stat : String -> String -> Html Msg
stat label value =
    div [ class "lap-stat" ]
        [ span [ class "lap-stat-label" ] [ text label ]
        , span [ class "lap-stat-value" ] [ text value ]
        ]


formatMs : Int -> String
formatMs ms =
    let
        minutes =
            ms // 60000

        seconds =
            (ms // 1000) |> modBy 60

        tenths =
            (ms // 100) |> modBy 10

        pad n =
            if n < 10 then
                "0" ++ String.fromInt n

            else
                String.fromInt n
    in
    pad minutes ++ ":" ++ pad seconds ++ "." ++ String.fromInt tenths

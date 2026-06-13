module Simulator exposing (main)

import BrokerPort exposing (Inbound(..), brokerIn, decode, ready)
import Browser
import Html exposing (Html, div, small, span, text)
import Html.Attributes exposing (class, classList, style, title)
import Html.Keyed
import Json.Decode as Decode exposing (bool, field, int, list, string)


{-| Simulator is the radial-network member of the contract-simulator card. It
owns no logic — it renders the latest simnet Frame the Go server pushes over the
"sim-frame" SSE event: the Go node at the centre, five surfaces on a ring, and a
packet flying out for whatever just happened (a green deliver, a red drop that
falls short, a blue duplicate). Surface colour tracks convergence; a partitioned
surface goes dashed and grey. The invariant verdict lives in the Datastar panel
beside it — both fed the very same frames.
-}
type alias Surface =
    { label : String
    , kind : String
    , status : String
    , version : Int
    , bufUsed : Int
    , bufCap : Int
    }


type alias Action =
    { kind : String
    , target : Int
    , label : String
    }


type alias Frame =
    { seed : Int
    , step : Int
    , total : Int
    , action : Action
    , auth : Int
    , surfaces : List Surface
    , converged : Bool
    , final : Bool
    , violated : Bool
    }


type alias Model =
    { frame : Maybe Frame }


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
    ( { frame = Nothing }, ready )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        BrokerIn raw ->
            case decode raw of
                Sse "sim-frame" data ->
                    case Decode.decodeValue frameDecoder data of
                        Ok f ->
                            ( { model | frame = Just f }, Cmd.none )

                        Err _ ->
                            ( model, Cmd.none )

                _ ->
                    ( model, Cmd.none )



-- GEOMETRY


cx : Float
cx =
    190


cy : Float
cy =
    150


radius : Float
radius =
    112


angleDeg : Int -> Int -> Float
angleDeg n i =
    -90 + 360 * toFloat i / toFloat (max 1 n)


bridgeR : Float
bridgeR =
    70


{-| at returns the (x, y) on surface i's spoke at distance r from the centre,
so nodes and bridge waypoints sit on the same line the packet travels.
-}
at : Int -> Int -> Float -> ( Float, Float )
at n i r =
    ( cx + r * cos (degrees (angleDeg n i))
    , cy + r * sin (degrees (angleDeg n i))
    )


px : Float -> String
px f =
    String.fromFloat f ++ "px"



-- VIEW


view : Model -> Html Msg
view model =
    case model.frame of
        Nothing ->
            div [ class "sim-stage" ]
                [ div [ class "muted", style "margin" "auto" ] [ text "Waiting for the first frame…" ] ]

        Just f ->
            let
                n =
                    List.length f.surfaces

                links =
                    List.indexedMap (linkPair f n) f.surfaces

                bridges =
                    List.indexedMap (bridgePair f n) f.surfaces

                nodes =
                    List.indexedMap (nodePair n) f.surfaces
            in
            div [ class "sim-viz" ]
                [ Html.Keyed.node "div"
                    [ class "sim-stage" ]
                    (links ++ bridges ++ nodes ++ [ ( serverKey f, serverView f ) ])
                , pipelineView f
                ]


linkPair : Frame -> Int -> Int -> Surface -> ( String, Html Msg )
linkPair f n i s =
    ( "link-" ++ String.fromInt i
    , Html.Keyed.node "div"
        [ classList
            [ ( "sim-link", True )
            , ( "down", s.status == "partitioned" )
            , ( "ds", s.kind == "datastar" )
            ]
        , style "left" (px cx)
        , style "top" (px (cy - 1))
        , style "width" (px radius)
        , style "transform" ("rotate(" ++ String.fromFloat (angleDeg n i) ++ "deg)")
        ]
        (case packetKind f i of
            Just kind ->
                [ ( "p-" ++ String.fromInt f.step, div [ class ("sim-packet " ++ kind) ] [] ) ]

            Nothing ->
                []
        )
    )


{-| packetKind returns the packet animation to play on link i this frame, or
Nothing when nothing flies to that surface.
-}
packetKind : Frame -> Int -> Maybe String
packetKind f i =
    if f.action.target /= i then
        Nothing

    else
        case f.action.kind of
            "deliver" ->
                Just "deliver"

            "reconnect" ->
                Just "deliver"

            "duplicate" ->
                Just "duplicate"

            "drop" ->
                Just "drop"

            "gap" ->
                Just "gap"

            _ ->
                Nothing


nodePair : Int -> Int -> Surface -> ( String, Html Msg )
nodePair n i s =
    let
        ( x, y ) =
            at n i radius
    in
    ( "node-" ++ String.fromInt i
    , div
        [ class ("sim-node " ++ s.status ++ " kind-" ++ s.kind)
        , style "left" (px (x - 23))
        , style "top" (px (y - 23))
        ]
        [ span [ class ("sim-kindbadge " ++ s.kind) ] [ text (kindShort s.kind) ]
        , small [] [ text ("v" ++ String.fromInt s.version) ]
        , bufpips s
        ]
    )


{-| bridgePair draws the per-surface bridge.js waypoint on the spoke. Each
independent subscriber holds its own EventSource + broker, so every surface is
reached through its own bridge. It lights up when this frame's event is heading
to that surface, and dims when the surface is partitioned (EventSource down).
-}
bridgePair : Frame -> Int -> Int -> Surface -> ( String, Html Msg )
bridgePair f n i s =
    if s.kind == "datastar" then
        -- Datastar has its own SSE stream and patches the DOM directly; it
        -- never passes through bridge.js, so its spoke carries no broker.
        ( "bridge-" ++ String.fromInt i, text "" )

    else
        let
            ( x, y ) =
                at n i bridgeR
        in
        ( "bridge-" ++ String.fromInt i
        , div
            [ classList
                [ ( "sim-bridge", True )
                , ( "active", f.action.target == i )
                , ( "down", s.status == "partitioned" )
                ]
            , style "left" (px (x - 12))
            , style "top" (px (y - 12))
            , title "bridge.js (browser broker)"
            ]
            [ text "JS" ]
        )


{-| pipelineView names every function a state change flows through and lights
the stage in play this frame: a drop happens at the Broadcaster, a duplicate on
the SSE wire, a partition/reconnect at bridge.js, a gap at the island port, a
deliver at the surface.
-}
pipelineView : Frame -> Html Msg
pipelineView f =
    let
        role =
            activeRole f.action.kind

        tone =
            stageTone f.action.kind

        stages =
            pipelineFor (targetKind f)
    in
    div [ class "sim-pipeline" ]
        (stages
            |> List.indexedMap
                (\idx ( r, label ) ->
                    let
                        cls =
                            if r == role then
                                "sim-stage-chip active " ++ tone

                            else
                                "sim-stage-chip"
                    in
                    [ if idx > 0 then
                        span [ class "sim-stage-arrow" ] [ text "→" ]

                      else
                        text ""
                    , span [ class cls ] [ text label ]
                    ]
                )
            |> List.concat
        )


{-| targetKind is the transport of the surface this frame's event is heading to,
which selects the pipeline shown (Elm, HTMX, or Datastar each differ).
-}
targetKind : Frame -> String
targetKind f =
    if f.action.target < 0 then
        "elm"

    else
        List.drop f.action.target f.surfaces
            |> List.head
            |> Maybe.map .kind
            |> Maybe.withDefault "elm"


{-| pipelineFor names the functions a state change flows through for each
transport. Elm and HTMX share the broker (bridge.js) but differ at the end —
Elm receives a push on its port, HTMX is nudged and pulls a fresh fragment.
Datastar runs its own SSE stream straight to the DOM, with no broker.
-}
pipelineFor : String -> List ( String, String )
pipelineFor kind =
    case kind of
        "datastar" ->
            [ ( "state", "Go state" )
            , ( "fanout", "Broadcaster" )
            , ( "transport", "Datastar SSE" )
            , ( "apply", "patch signals" )
            , ( "render", "DOM" )
            ]

        "htmx" ->
            [ ( "state", "Go state" )
            , ( "fanout", "Broadcaster" )
            , ( "transport", "SSE wire" )
            , ( "hub", "bridge.js" )
            , ( "apply", "htmx.trigger → GET" )
            , ( "render", "swap" )
            ]

        _ ->
            [ ( "state", "Go state" )
            , ( "fanout", "Broadcaster" )
            , ( "transport", "SSE wire" )
            , ( "hub", "bridge.js" )
            , ( "apply", "island port" )
            , ( "render", "render" )
            ]


{-| activeRole maps an action to the pipeline role lit this frame: a drop is at
the Broadcaster, a duplicate/partition on the transport, a gap at the apply
step, a deliver/reconnect/hydrate at render.
-}
activeRole : String -> String
activeRole kind =
    case kind of
        "write" ->
            "state"

        "drop" ->
            "fanout"

        "duplicate" ->
            "transport"

        "partition" ->
            "transport"

        "reconnect" ->
            "render"

        "gap" ->
            "apply"

        "deliver" ->
            "render"

        "hydrate" ->
            "render"

        _ ->
            ""


stageTone : String -> String
stageTone kind =
    case kind of
        "drop" ->
            "bad"

        "gap" ->
            "bad"

        "partition" ->
            "bad"

        "duplicate" ->
            "warn"

        "deliver" ->
            "ok"

        "reconnect" ->
            "ok"

        "hydrate" ->
            "ok"

        "write" ->
            "go"

        _ ->
            ""


bufpips : Surface -> Html Msg
bufpips s =
    div [ class "sim-bufpips" ]
        (List.range 1 s.bufCap
            |> List.map
                (\k ->
                    div [ classList [ ( "sim-bufpip", True ), ( "on", k <= s.bufUsed ) ] ] []
                )
        )


serverView : Frame -> Html Msg
serverView f =
    div
        [ classList [ ( "sim-server", True ), ( "write", f.action.kind == "write" ) ]
        , style "left" (px (cx - 38))
        , style "top" (px (cy - 38))
        ]
        [ div [] [ text "Go" ]
        , small [] [ text ("v" ++ String.fromInt f.auth) ]
        ]


{-| serverKey changes only on write frames, so the pulse animation restarts on a
write but the node persists (and so transitions smoothly) otherwise.
-}
serverKey : Frame -> String
serverKey f =
    if f.action.kind == "write" then
        "server-" ++ String.fromInt f.step

    else
        "server-idle"


kindShort : String -> String
kindShort kind =
    case kind of
        "elm" ->
            "ELM"

        "htmx" ->
            "HTMX"

        "datastar" ->
            "DS"

        _ ->
            "?"



-- DECODERS


surfaceDecoder : Decode.Decoder Surface
surfaceDecoder =
    Decode.map6 Surface
        (field "label" string)
        (field "kind" string)
        (field "status" string)
        (field "version" int)
        (field "bufferUsed" int)
        (field "bufferCap" int)


actionDecoder : Decode.Decoder Action
actionDecoder =
    Decode.map3 Action
        (field "kind" string)
        (field "surface" int)
        (field "label" string)


frameDecoder : Decode.Decoder Frame
frameDecoder =
    Decode.map8
        (\seed step total auth surfaces converged final violated ->
            \action -> Frame seed step total action auth surfaces converged final violated
        )
        (field "seed" int)
        (field "step" int)
        (field "total" int)
        (field "authVersion" int)
        (field "surfaces" (list surfaceDecoder))
        (field "converged" bool)
        (field "final" bool)
        (field "violated" bool)
        |> Decode.andThen (\partial -> Decode.map partial (field "action" actionDecoder))

module FiveWay exposing (main)

import BrokerPort exposing (Inbound(..), brokerIn, decode, ready, sendStateSet)
import Browser
import Html exposing (Html, button, div, h3, p, span, text)
import Html.Attributes exposing (class)
import Html.Events exposing (onClick)
import Json.Decode as Decode
import Json.Encode as Encode


type alias Lattice =
    { seq : Int
    , nodes : Int
    , edges : Int
    , selected : String
    , lastAction : String
    , lastSource : String
    }


type alias Model =
    { ready : Bool
    , lattice : Maybe Lattice
    }


type Msg
    = BrokerIn Decode.Value
    | AddNode
    | Promote
    | Reset


main : Program Decode.Value Model Msg
main =
    Browser.element
        { init = \_ -> ( { ready = False, lattice = Nothing }, ready )
        , update = update
        , view = view
        , subscriptions = \_ -> brokerIn BrokerIn
        }


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        BrokerIn value ->
            case decode value of
                BrokerReady ->
                    ( { model | ready = True }, Cmd.none )

                Sse "lattice-state" data ->
                    ( { model | lattice = Decode.decodeValue latticeDecoder data |> Result.toMaybe }, Cmd.none )

                _ ->
                    ( model, Cmd.none )

        AddNode ->
            ( model, sendCommand "add-node" "" )

        Promote ->
            ( model, sendCommand "promote" "" )

        Reset ->
            ( model, sendCommand "reset" "" )


sendCommand : String -> String -> Cmd Msg
sendCommand action node =
    sendStateSet "broker"
        "latticeCommand"
        (Encode.object
            [ ( "action", Encode.string action )
            , ( "node", Encode.string node )
            , ( "source", Encode.string "elm" )
            ]
        )


latticeDecoder : Decode.Decoder Lattice
latticeDecoder =
    Decode.map6 Lattice
        (Decode.field "seq" Decode.int)
        (Decode.field "nodes" (Decode.list Decode.value) |> Decode.map List.length)
        (Decode.field "edges" (Decode.list Decode.value) |> Decode.map List.length)
        (Decode.field "selected" Decode.string)
        (Decode.field "lastAction" Decode.string)
        (Decode.field "lastSource" Decode.string)


view : Model -> Html Msg
view model =
    div []
        [ h3 [] [ text "Elm command panel" ]
        , p []
            [ span [ class (if model.ready then "badge-ready" else "badge-waiting") ]
                [ text (if model.ready then "broker ready" else "waiting") ]
            ]
        , case model.lattice of
            Just lattice ->
                div []
                    [ p [] [ text ("seq " ++ String.fromInt lattice.seq ++ " / " ++ String.fromInt lattice.nodes ++ " nodes / " ++ String.fromInt lattice.edges ++ " edges") ]
                    , p [] [ text ("selected " ++ lattice.selected) ]
                    , p [] [ text ("last " ++ lattice.lastAction ++ " via " ++ lattice.lastSource) ]
                    ]

            Nothing ->
                p [] [ text "waiting for lattice-state SSE" ]
        , div [ class "elm-actions" ]
            [ button [ onClick AddNode ] [ text "Elm add node" ]
            , button [ onClick Promote ] [ text "Elm promote" ]
            , button [ onClick Reset ] [ text "Elm reset" ]
            ]
        ]

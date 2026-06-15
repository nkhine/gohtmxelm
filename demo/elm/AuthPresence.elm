module AuthPresence exposing (main)

import BrokerPort exposing (Inbound(..), brokerIn, decode, ready)
import Browser
import Html exposing (Html, div, span, text)
import Html.Attributes exposing (class)
import Json.Decode as Decode


type Status
    = Offline
    | Idle
    | Online


type alias Model =
    { status : Status
    , email : String
    , brokerReady : Bool
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
    ( { status = Offline, email = "", brokerReady = False }, ready )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        BrokerIn value ->
            case decode value of
                BrokerReady ->
                    ( { model | brokerReady = True }, Cmd.none )

                Sse "auth-presence" data ->
                    ( { model
                        | status = decodeStatus data
                        , email = decodeString "email" "" data
                      }
                    , Cmd.none
                    )

                _ ->
                    ( model, Cmd.none )


view : Model -> Html Msg
view model =
    div [ class "auth-presence-elm" ]
        [ div [ class "auth-presence-elm-main" ]
            [ span [ class ("auth-presence-elm-dot " ++ statusClass model.status) ] []
            , span [ class "auth-presence-elm-state" ] [ text (statusLabel model.status) ]
            ]
        , span [ class "auth-presence-elm-email" ] [ text (emailLabel model) ]
        ]


decodeStatus : Decode.Value -> Status
decodeStatus value =
    case decodeString "state" "offline" value of
        "online" ->
            Online

        "idle" ->
            Idle

        _ ->
            Offline


decodeString : String -> String -> Decode.Value -> String
decodeString name fallback value =
    Decode.decodeValue (Decode.field name Decode.string) value
        |> Result.withDefault fallback


statusClass : Status -> String
statusClass status =
    case status of
        Online ->
            "online"

        Idle ->
            "idle"

        Offline ->
            "offline"


statusLabel : Status -> String
statusLabel status =
    case status of
        Online ->
            "online"

        Idle ->
            "idle"

        Offline ->
            "logged out"


emailLabel : Model -> String
emailLabel model =
    if model.email == "" then
        "no active session"

    else
        model.email

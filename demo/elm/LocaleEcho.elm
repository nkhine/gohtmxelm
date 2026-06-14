module LocaleEcho exposing (main)

import BrokerPort exposing (brokerIn, ready)
import Browser
import Dict exposing (Dict)
import Html exposing (Html, div, dl, h4, p, text)
import Html.Attributes exposing (class)
import Json.Decode as Decode


type alias Model =
    { title : String
    , sample : String
    , locale : String
    , timezone : String
    , currency : String
    , messages : Dict String String
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
init flags =
    ( { title = stringField "title" "Elm island flags" flags
      , sample = stringField "sample" "" flags
      , locale = stringField "locale" "en-GB" flags
      , timezone = stringField "timezone" "" flags
      , currency = stringField "currency" "" flags
      , messages = dictField "messages" flags
      }
    , ready
    )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        BrokerIn _ ->
            ( model, Cmd.none )


view : Model -> Html Msg
view model =
    div [ class "locale-echo" ]
        [ h4 [] [ text model.title ]
        , p [] [ text model.sample ]
        , dl [ class "localization-facts" ]
            [ fact (t model "localization.locale_label") model.locale
            , fact (t model "localization.timezone_label") model.timezone
            , fact (t model "localization.currency_label") model.currency
            ]
        ]


fact : String -> String -> Html Msg
fact label value =
    div [] [ Html.dt [] [ text label ], Html.dd [] [ text value ] ]


t : Model -> String -> String
t model key =
    Dict.get key model.messages
        |> Maybe.withDefault ("[" ++ key ++ "]")


stringField : String -> String -> Decode.Value -> String
stringField name fallback value =
    Decode.decodeValue (Decode.field name Decode.string) value
        |> Result.withDefault fallback


dictField : String -> Decode.Value -> Dict String String
dictField name value =
    Decode.decodeValue (Decode.field name (Decode.dict Decode.string)) value
        |> Result.withDefault Dict.empty

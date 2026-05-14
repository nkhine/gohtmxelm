module Main exposing (main)

import Browser
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import Json.Decode as Decode


-- ── TYPES ─────────────────────────────────────────────────────────────────────


type Step
    = StepInvite
    | StepOtp
    | StepAddress
    | StepBank
    | StepDocuments
    | StepConfirmation
    | StepComplete


type AddressField
    = AFFirstName
    | AFLastName
    | AFDob
    | AFLine1
    | AFLine2
    | AFCity
    | AFCounty
    | AFPostcode
    | AFCountry


type BankField
    = BFHolderName
    | BFBankName
    | BFCurrency
    | BFAccountNumber
    | BFSortCode
    | BFIban
    | BFSwiftBic


type DocumentType
    = Passport
    | BankStatement
    | UtilityBill
    | DrivingLicence
    | OtherDoc


type UploadStatus
    = NotUploaded
    | Uploaded


type alias Document =
    { id : Int
    , fileName : String
    , docType : Maybe DocumentType
    , status : UploadStatus
    }


type alias Address =
    { firstName : String
    , lastName : String
    , dob : String
    , line1 : String
    , line2 : String
    , city : String
    , county : String
    , postcode : String
    , country : String
    }


type alias BankDetails =
    { holderName : String
    , bankName : String
    , currency : String
    , accountNumber : String
    , sortCode : String
    , iban : String
    , swiftBic : String
    }


-- ── MODEL ─────────────────────────────────────────────────────────────────────


type alias Model =
    { step : Step
    , editingFrom : Maybe Step

    -- Invite
    , inviteCode : String
    , inviteError : Maybe String
    , inviteVerified : Bool

    -- OTP
    , mobile : String
    , email : String
    , mobileOtpSent : Bool
    , emailOtpSent : Bool
    , mobileOtp : String
    , emailOtp : String
    , mobileVerified : Bool
    , emailVerified : Bool
    , otpError : Maybe String

    -- Address
    , address : Address
    , addressErrors : List String
    , addressSaved : Bool

    -- Bank
    , bank : BankDetails
    , bankErrors : List String
    , bankSaved : Bool

    -- Documents
    , documents : List Document
    , nextDocId : Int
    , docErrors : List String
    , documentsSaved : Bool

    , submitted : Bool
    }


emptyAddress : Address
emptyAddress =
    { firstName = ""
    , lastName = ""
    , dob = ""
    , line1 = ""
    , line2 = ""
    , city = ""
    , county = ""
    , postcode = ""
    , country = ""
    }


emptyBank : BankDetails
emptyBank =
    { holderName = ""
    , bankName = ""
    , currency = "GBP"
    , accountNumber = ""
    , sortCode = ""
    , iban = ""
    , swiftBic = ""
    }


init : () -> ( Model, Cmd Msg )
init _ =
    ( { step = StepInvite
      , editingFrom = Nothing
      , inviteCode = ""
      , inviteError = Nothing
      , inviteVerified = False
      , mobile = ""
      , email = ""
      , mobileOtpSent = False
      , emailOtpSent = False
      , mobileOtp = ""
      , emailOtp = ""
      , mobileVerified = False
      , emailVerified = False
      , otpError = Nothing
      , address = emptyAddress
      , addressErrors = []
      , addressSaved = False
      , bank = emptyBank
      , bankErrors = []
      , bankSaved = False
      , documents = []
      , nextDocId = 1
      , docErrors = []
      , documentsSaved = False
      , submitted = False
      }
    , Cmd.none
    )


-- ── MSG ───────────────────────────────────────────────────────────────────────


type Msg
    = SetInviteCode String
    | SubmitInviteCode
    | SetMobile String
    | SetEmail String
    | SendMobileOtp
    | SendEmailOtp
    | SetMobileOtp String
    | SetEmailOtp String
    | VerifyMobileOtp
    | VerifyEmailOtp
    | SetAddressField AddressField String
    | SubmitAddress
    | SetBankField BankField String
    | SubmitBank
    | AddDocument
    | RemoveDocument Int
    | SetDocFile Int String
    | SetDocType Int (Maybe DocumentType)
    | MockUpload Int
    | SubmitDocuments
    | NavigateToStep Step
    | EditSection Step
    | SubmitOnboarding


-- ── HELPERS ───────────────────────────────────────────────────────────────────


isDone : Step -> Model -> Bool
isDone step model =
    case step of
        StepInvite -> model.inviteVerified
        StepOtp -> model.mobileVerified && model.emailVerified
        StepAddress -> model.addressSaved
        StepBank -> model.bankSaved
        StepDocuments -> model.documentsSaved
        StepConfirmation -> model.submitted
        StepComplete -> False


isAccessible : Step -> Model -> Bool
isAccessible step model =
    step == model.step || isDone step model


afterSave : Step -> Model -> Step
afterSave defaultNext model =
    Maybe.withDefault defaultNext model.editingFrom


-- ── VALIDATION ────────────────────────────────────────────────────────────────


req : String -> String -> Maybe String
req label value =
    if String.isEmpty (String.trim value) then
        Just (label ++ " is required")
    else
        Nothing


validateAddress : Address -> List String
validateAddress a =
    List.filterMap identity
        [ req "First name" a.firstName
        , req "Last name" a.lastName
        , req "Date of birth" a.dob
        , req "Address line 1" a.line1
        , req "City" a.city
        , req "Postcode" a.postcode
        , req "Country" a.country
        ]


validateBank : BankDetails -> List String
validateBank b =
    let
        base =
            List.filterMap identity
                [ req "Account holder name" b.holderName
                , req "Bank name" b.bankName
                ]

        currencyErrors =
            if b.currency == "GBP" then
                List.filterMap identity
                    [ req "Account number" b.accountNumber
                    , req "Sort code" b.sortCode
                    ]
            else
                List.filterMap identity
                    [ req "IBAN" b.iban
                    , req "SWIFT / BIC" b.swiftBic
                    ]
    in
    base ++ currencyErrors


validateDocuments : List Document -> List String
validateDocuments docs =
    if List.isEmpty docs then
        [ "Please add at least one document" ]
    else
        let
            incomplete =
                List.filter
                    (\d -> String.isEmpty d.fileName || d.docType == Nothing || d.status /= Uploaded)
                    docs
        in
        if List.isEmpty incomplete then
            []
        else
            [ "Each document needs a file, a document type, and must be uploaded" ]


-- ── UPDATE ────────────────────────────────────────────────────────────────────


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        SetInviteCode s ->
            ( { model | inviteCode = s, inviteError = Nothing }, Cmd.none )

        SubmitInviteCode ->
            if String.toUpper (String.trim model.inviteCode) == "INVITE-123456" then
                ( { model
                    | inviteVerified = True
                    , inviteError = Nothing
                    , step = afterSave StepOtp model
                    , editingFrom = Nothing
                  }
                , Cmd.none
                )
            else
                ( { model | inviteError = Just "Invalid invite code. Try INVITE-123456." }, Cmd.none )

        SetMobile s ->
            ( { model | mobile = s }, Cmd.none )

        SetEmail s ->
            ( { model | email = s }, Cmd.none )

        SendMobileOtp ->
            -- Real implementation: POST /api/otp/send { channel: "sms", to: model.mobile }
            ( { model | mobileOtpSent = True }, Cmd.none )

        SendEmailOtp ->
            -- Real implementation: POST /api/otp/send { channel: "email", to: model.email }
            ( { model | emailOtpSent = True }, Cmd.none )

        SetMobileOtp s ->
            ( { model | mobileOtp = s, otpError = Nothing }, Cmd.none )

        SetEmailOtp s ->
            ( { model | emailOtp = s, otpError = Nothing }, Cmd.none )

        VerifyMobileOtp ->
            if model.mobileOtp == "123456" then
                let
                    m =
                        { model | mobileVerified = True, otpError = Nothing }
                in
                if m.emailVerified then
                    ( { m | step = afterSave StepAddress m, editingFrom = Nothing }, Cmd.none )
                else
                    ( m, Cmd.none )
            else
                ( { model | otpError = Just "Incorrect OTP — the mock code is 123456." }, Cmd.none )

        VerifyEmailOtp ->
            if model.emailOtp == "123456" then
                let
                    m =
                        { model | emailVerified = True, otpError = Nothing }
                in
                if m.mobileVerified then
                    ( { m | step = afterSave StepAddress m, editingFrom = Nothing }, Cmd.none )
                else
                    ( m, Cmd.none )
            else
                ( { model | otpError = Just "Incorrect OTP — the mock code is 123456." }, Cmd.none )

        SetAddressField field s ->
            let
                a =
                    model.address

                newAddr =
                    case field of
                        AFFirstName -> { a | firstName = s }
                        AFLastName -> { a | lastName = s }
                        AFDob -> { a | dob = s }
                        AFLine1 -> { a | line1 = s }
                        AFLine2 -> { a | line2 = s }
                        AFCity -> { a | city = s }
                        AFCounty -> { a | county = s }
                        AFPostcode -> { a | postcode = s }
                        AFCountry -> { a | country = s }
            in
            ( { model | address = newAddr, addressErrors = [] }, Cmd.none )

        SubmitAddress ->
            let
                errors =
                    validateAddress model.address
            in
            if List.isEmpty errors then
                ( { model
                    | addressSaved = True
                    , addressErrors = []
                    , step = afterSave StepBank model
                    , editingFrom = Nothing
                  }
                , Cmd.none
                )
            else
                ( { model | addressErrors = errors }, Cmd.none )

        SetBankField field s ->
            let
                b =
                    model.bank

                newBank =
                    case field of
                        BFHolderName -> { b | holderName = s }
                        BFBankName -> { b | bankName = s }
                        BFCurrency -> { b | currency = s }
                        BFAccountNumber -> { b | accountNumber = s }
                        BFSortCode -> { b | sortCode = s }
                        BFIban -> { b | iban = s }
                        BFSwiftBic -> { b | swiftBic = s }
            in
            ( { model | bank = newBank, bankErrors = [] }, Cmd.none )

        SubmitBank ->
            let
                errors =
                    validateBank model.bank
            in
            if List.isEmpty errors then
                ( { model
                    | bankSaved = True
                    , bankErrors = []
                    , step = afterSave StepDocuments model
                    , editingFrom = Nothing
                  }
                , Cmd.none
                )
            else
                ( { model | bankErrors = errors }, Cmd.none )

        AddDocument ->
            let
                doc =
                    { id = model.nextDocId
                    , fileName = ""
                    , docType = Nothing
                    , status = NotUploaded
                    }
            in
            ( { model | documents = model.documents ++ [ doc ], nextDocId = model.nextDocId + 1 }
            , Cmd.none
            )

        RemoveDocument docId ->
            ( { model | documents = List.filter (\d -> d.id /= docId) model.documents }, Cmd.none )

        SetDocFile docId name ->
            ( { model
                | documents =
                    List.map
                        (\d ->
                            if d.id == docId then
                                { d | fileName = name, status = NotUploaded }
                            else
                                d
                        )
                        model.documents
              }
            , Cmd.none
            )

        SetDocType docId dt ->
            ( { model
                | documents =
                    List.map
                        (\d ->
                            if d.id == docId then
                                { d | docType = dt }
                            else
                                d
                        )
                        model.documents
              }
            , Cmd.none
            )

        MockUpload docId ->
            -- Real implementation: POST /api/documents/upload (multipart/form-data)
            ( { model
                | documents =
                    List.map
                        (\d ->
                            if d.id == docId then
                                { d | status = Uploaded }
                            else
                                d
                        )
                        model.documents
              }
            , Cmd.none
            )

        SubmitDocuments ->
            let
                errors =
                    validateDocuments model.documents
            in
            if List.isEmpty errors then
                ( { model
                    | documentsSaved = True
                    , docErrors = []
                    , step = afterSave StepConfirmation model
                    , editingFrom = Nothing
                  }
                , Cmd.none
                )
            else
                ( { model | docErrors = errors }, Cmd.none )

        NavigateToStep target ->
            if isAccessible target model then
                ( { model | step = target, editingFrom = Nothing }, Cmd.none )
            else
                ( model, Cmd.none )

        EditSection section ->
            -- Reset OTP state when editing so user must re-verify with any new details.
            let
                m =
                    if section == StepOtp then
                        { model
                            | mobileOtpSent = False
                            , emailOtpSent = False
                            , mobileOtp = ""
                            , emailOtp = ""
                            , mobileVerified = False
                            , emailVerified = False
                            , otpError = Nothing
                        }
                    else
                        model
            in
            ( { m | step = section, editingFrom = Just StepConfirmation }, Cmd.none )

        SubmitOnboarding ->
            -- Real implementation: POST /api/onboarding/submit
            ( { model | submitted = True, step = StepComplete }, Cmd.none )


-- ── VIEW ──────────────────────────────────────────────────────────────────────


view : Model -> Html Msg
view model =
    div [ class "app" ]
        [ div [ class "container" ]
            (if model.step == StepComplete then
                [ viewComplete ]
             else
                [ h1 [ class "app-title" ] [ text "Payee Onboarding" ]
                , p [ class "app-subtitle" ] [ text "Complete each step to onboard as a payee." ]
                , viewStepper model
                , viewCurrentStep model
                ]
            )
        ]


viewCurrentStep : Model -> Html Msg
viewCurrentStep model =
    case model.step of
        StepInvite -> viewInvite model
        StepOtp -> viewOtp model
        StepAddress -> viewAddress model
        StepBank -> viewBank model
        StepDocuments -> viewDocuments model
        StepConfirmation -> viewConfirmation model
        StepComplete -> viewComplete


-- ── STEPPER ───────────────────────────────────────────────────────────────────


viewStepper : Model -> Html Msg
viewStepper model =
    let
        steps =
            [ ( StepInvite, "1", "Invite" )
            , ( StepOtp, "2", "Verify" )
            , ( StepAddress, "3", "Address" )
            , ( StepBank, "4", "Bank" )
            , ( StepDocuments, "5", "Documents" )
            , ( StepConfirmation, "6", "Review" )
            ]

        viewItem ( step, num, lbl ) =
            let
                done =
                    isDone step model

                active =
                    model.step == step

                cls =
                    "step-item"
                        ++ (if done then " done" else "")
                        ++ (if active then " active" else "")

                clickAttrs =
                    if done && not active then
                        [ onClick (NavigateToStep step) ]
                    else
                        []
            in
            div (class cls :: clickAttrs)
                [ div [ class "step-circle" ]
                    [ text (if done then "✓" else num) ]
                , span [ class "step-label" ] [ text lbl ]
                ]
    in
    div [ class "stepper" ]
        (List.map viewItem steps)


-- ── STEP: INVITE ──────────────────────────────────────────────────────────────


viewInvite : Model -> Html Msg
viewInvite model =
    div [ class "card" ]
        [ h2 [ class "card-title" ] [ text "Invite Code" ]
        , p [ class "card-subtitle" ] [ text "Enter your invite code to begin. (Mock code: INVITE-123456)" ]
        , div [ class "form-group" ]
            [ label [ class "form-label" ] [ text "Invite code" ]
            , input
                [ class "form-input"
                , type_ "text"
                , placeholder "INVITE-XXXXXX"
                , value model.inviteCode
                , onInput SetInviteCode
                , onEnter SubmitInviteCode
                ]
                []
            , viewFieldError model.inviteError
            ]
        , div [ class "form-actions" ]
            [ button
                [ class "btn btn-primary"
                , onClick SubmitInviteCode
                , disabled (String.isEmpty (String.trim model.inviteCode))
                ]
                [ text "Verify Code" ]
            ]
        ]


-- ── STEP: OTP ─────────────────────────────────────────────────────────────────


viewOtp : Model -> Html Msg
viewOtp model =
    div [ class "card" ]
        [ h2 [ class "card-title" ] [ text "Verify Identity" ]
        , p [ class "card-subtitle" ] [ text "Verify your mobile and email address. (Mock OTP: 123456)" ]
        , viewEditingBanner model
        , viewGlobalError model.otpError
        , viewOtpChannel
            { heading = "Mobile verification"
            , inputType = "tel"
            , placeholder = "+44 7700 000000"
            , value = model.mobile
            , onChangeContact = SetMobile
            , onSend = SendMobileOtp
            , sent = model.mobileOtpSent
            , verified = model.mobileVerified
            , verifiedLabel = model.mobile
            , otpValue = model.mobileOtp
            , onChangeOtp = SetMobileOtp
            , onVerify = VerifyMobileOtp
            }
        , viewOtpChannel
            { heading = "Email verification"
            , inputType = "email"
            , placeholder = "you@example.com"
            , value = model.email
            , onChangeContact = SetEmail
            , onSend = SendEmailOtp
            , sent = model.emailOtpSent
            , verified = model.emailVerified
            , verifiedLabel = model.email
            , otpValue = model.emailOtp
            , onChangeOtp = SetEmailOtp
            , onVerify = VerifyEmailOtp
            }
        ]


type alias OtpChannelConfig =
    { heading : String
    , inputType : String
    , placeholder : String
    , value : String
    , onChangeContact : String -> Msg
    , onSend : Msg
    , sent : Bool
    , verified : Bool
    , verifiedLabel : String
    , otpValue : String
    , onChangeOtp : String -> Msg
    , onVerify : Msg
    }


viewOtpChannel : OtpChannelConfig -> Html Msg
viewOtpChannel cfg =
    div [ class "otp-section" ]
        [ h3 [] [ text cfg.heading ]
        , if cfg.verified then
            div [ class "verified-badge" ] [ text ("✓ Verified — " ++ cfg.verifiedLabel) ]
          else
            div []
                [ div [ class "otp-row" ]
                    [ div [ class "form-group" ]
                        [ label [ class "form-label" ] [ text cfg.heading ]
                        , input
                            [ class "form-input"
                            , type_ cfg.inputType
                            , placeholder cfg.placeholder
                            , value cfg.value
                            , onInput cfg.onChangeContact
                            ]
                            []
                        ]
                    , button
                        [ class "btn btn-secondary"
                        , onClick cfg.onSend
                        , disabled (String.isEmpty (String.trim cfg.value) || cfg.sent)
                        ]
                        [ text
                            (if cfg.sent then
                                "Code sent ✓"
                             else
                                "Send OTP"
                            )
                        ]
                    ]
                , if cfg.sent then
                    div [ class "otp-row", style "margin-top" "0.5rem" ]
                        [ div [ class "form-group" ]
                            [ label [ class "form-label" ] [ text "Enter OTP" ]
                            , input
                                [ class "form-input"
                                , type_ "text"
                                , placeholder "123456"
                                , value cfg.otpValue
                                , onInput cfg.onChangeOtp
                                , maxlength 6
                                , style "letter-spacing" "0.2em"
                                , style "font-size" "1.1rem"
                                ]
                                []
                            ]
                        , button
                            [ class "btn btn-primary"
                            , onClick cfg.onVerify
                            , disabled (String.length cfg.otpValue /= 6)
                            ]
                            [ text "Verify" ]
                        ]
                  else
                    text ""
                ]
        ]


-- ── STEP: ADDRESS ─────────────────────────────────────────────────────────────


viewAddress : Model -> Html Msg
viewAddress model =
    div [ class "card" ]
        [ h2 [ class "card-title" ] [ text "Personal Details & Address" ]
        , p [ class "card-subtitle" ] [ text "Enter your name, date of birth and residential address." ]
        , viewEditingBanner model
        , viewErrors model.addressErrors
        , div [ class "form-row" ]
            [ formField "First name" (textInput "Jane" model.address.firstName (SetAddressField AFFirstName))
            , formField "Last name" (textInput "Smith" model.address.lastName (SetAddressField AFLastName))
            ]
        , formField "Date of birth"
            (input
                [ class "form-input"
                , type_ "date"
                , value model.address.dob
                , onInput (SetAddressField AFDob)
                ]
                []
            )
        , hr [ class "section-divider" ] []
        , formField "Address line 1" (textInput "123 High Street" model.address.line1 (SetAddressField AFLine1))
        , formField "Address line 2 (optional)" (textInput "Flat 4" model.address.line2 (SetAddressField AFLine2))
        , div [ class "form-row" ]
            [ formField "City" (textInput "London" model.address.city (SetAddressField AFCity))
            , formField "County / State" (textInput "Greater London" model.address.county (SetAddressField AFCounty))
            ]
        , div [ class "form-row" ]
            [ formField "Postcode" (textInput "SW1A 1AA" model.address.postcode (SetAddressField AFPostcode))
            , formField "Country" (textInput "United Kingdom" model.address.country (SetAddressField AFCountry))
            ]
        , div [ class "form-actions" ]
            [ backButton StepOtp model
            , button [ class "btn btn-primary", onClick SubmitAddress ]
                [ text (saveLabel "Save & Continue" model) ]
            ]
        ]


-- ── STEP: BANK DETAILS ────────────────────────────────────────────────────────


viewBank : Model -> Html Msg
viewBank model =
    let
        b =
            model.bank

        isGbp =
            b.currency == "GBP"
    in
    div [ class "card" ]
        [ h2 [ class "card-title" ] [ text "Bank Details" ]
        , p [ class "card-subtitle" ] [ text "Provide the bank account where payments will be sent." ]
        , viewEditingBanner model
        , viewErrors model.bankErrors
        , div [ class "form-row" ]
            [ formField "Account holder name" (textInput "Jane Smith" b.holderName (SetBankField BFHolderName))
            , formField "Bank name" (textInput "Barclays" b.bankName (SetBankField BFBankName))
            ]
        , formField "Currency"
            (select
                [ class "form-input"
                , onInput (SetBankField BFCurrency)
                ]
                [ option [ value "GBP", selected (b.currency == "GBP") ] [ text "GBP — British Pound" ]
                , option [ value "EUR", selected (b.currency == "EUR") ] [ text "EUR — Euro" ]
                , option [ value "USD", selected (b.currency == "USD") ] [ text "USD — US Dollar" ]
                , option [ value "CHF", selected (b.currency == "CHF") ] [ text "CHF — Swiss Franc" ]
                , option [ value "AED", selected (b.currency == "AED") ] [ text "AED — UAE Dirham" ]
                ]
            )
        , hr [ class "section-divider" ] []
        , if isGbp then
            div []
                [ div [ class "info-box" ] [ text "GBP accounts require an account number and sort code." ]
                , div [ class "form-row" ]
                    [ formField "Account number" (textInput "12345678" b.accountNumber (SetBankField BFAccountNumber))
                    , formField "Sort code" (textInput "12-34-56" b.sortCode (SetBankField BFSortCode))
                    ]
                ]
          else
            div []
                [ div [ class "info-box" ] [ text "International accounts require an IBAN and SWIFT/BIC code." ]
                , formField "IBAN" (textInput "GB29 NWBK 6016 1331 9268 19" b.iban (SetBankField BFIban))
                , formField "SWIFT / BIC" (textInput "NWBKGB2L" b.swiftBic (SetBankField BFSwiftBic))
                ]
        , div [ class "form-actions" ]
            [ backButton StepAddress model
            , button [ class "btn btn-primary", onClick SubmitBank ]
                [ text (saveLabel "Save & Continue" model) ]
            ]
        ]


-- ── STEP: DOCUMENTS ───────────────────────────────────────────────────────────


viewDocuments : Model -> Html Msg
viewDocuments model =
    div [ class "card" ]
        [ h2 [ class "card-title" ] [ text "Document Uploads" ]
        , p [ class "card-subtitle" ] [ text "Upload supporting documents. Select a file, choose a type, then click Upload." ]
        , viewEditingBanner model
        , viewErrors model.docErrors
        , div [ class "doc-list" ]
            (List.map viewDocRow model.documents)
        , button [ class "btn btn-ghost btn-sm", onClick AddDocument ]
            [ text "+ Add document" ]
        , div [ class "form-actions" ]
            [ backButton StepBank model
            , button
                [ class "btn btn-primary"
                , onClick SubmitDocuments
                , disabled (List.isEmpty model.documents)
                ]
                [ text (saveLabel "Save & Continue" model) ]
            ]
        ]


viewDocRow : Document -> Html Msg
viewDocRow doc =
    div [ class "doc-row" ]
        [ div []
            [ input
                [ type_ "file"
                , class "form-input"
                , style "padding" "0.3rem"
                , onFileChange (SetDocFile doc.id)
                ]
                []
            , if not (String.isEmpty doc.fileName) then
                div [ class "doc-filename" ] [ text doc.fileName ]
              else
                text ""
            ]
        , viewDocTypeSelect doc.id doc.docType
        , case doc.status of
            Uploaded ->
                div [ class "upload-done" ] [ text "✓ Uploaded" ]

            NotUploaded ->
                button
                    [ class "btn btn-secondary btn-sm"
                    , onClick (MockUpload doc.id)
                    , disabled (String.isEmpty doc.fileName || doc.docType == Nothing)
                    ]
                    [ text "Upload" ]
        , button
            [ class "btn btn-danger btn-sm"
            , onClick (RemoveDocument doc.id)
            ]
            [ text "✕" ]
        ]


viewDocTypeSelect : Int -> Maybe DocumentType -> Html Msg
viewDocTypeSelect docId current =
    let
        parseDocType s =
            case s of
                "passport" -> Just Passport
                "bank-statement" -> Just BankStatement
                "utility-bill" -> Just UtilityBill
                "driving-licence" -> Just DrivingLicence
                "other" -> Just OtherDoc
                _ -> Nothing
    in
    select
        [ class "form-input"
        , onInput (\s -> SetDocType docId (parseDocType s))
        ]
        [ option [ value "", selected (current == Nothing) ] [ text "Document type…" ]
        , option [ value "passport", selected (current == Just Passport) ] [ text "Passport" ]
        , option [ value "bank-statement", selected (current == Just BankStatement) ] [ text "Bank Statement" ]
        , option [ value "utility-bill", selected (current == Just UtilityBill) ] [ text "Utility Bill" ]
        , option [ value "driving-licence", selected (current == Just DrivingLicence) ] [ text "Driving Licence" ]
        , option [ value "other", selected (current == Just OtherDoc) ] [ text "Other" ]
        ]


-- ── STEP: CONFIRMATION ────────────────────────────────────────────────────────


viewConfirmation : Model -> Html Msg
viewConfirmation model =
    let
        a =
            model.address

        b =
            model.bank
    in
    div [ class "card" ]
        [ h2 [ class "card-title" ] [ text "Review & Submit" ]
        , p [ class "card-subtitle" ] [ text "Check everything looks correct before submitting." ]

        -- Invite
        , div [ class "confirm-section" ]
            [ div [ class "confirm-header" ]
                [ h3 [] [ text "Invite" ]
                , button [ class "btn btn-sm btn-ghost", onClick (EditSection StepInvite) ] [ text "Edit" ]
                ]
            , div [ class "confirm-grid" ]
                [ confirmField "Invite code" model.inviteCode ]
            ]

        -- Verification
        , div [ class "confirm-section" ]
            [ div [ class "confirm-header" ]
                [ h3 [] [ text "Verification" ]
                , button [ class "btn btn-sm btn-ghost", onClick (EditSection StepOtp) ] [ text "Edit" ]
                ]
            , div [ class "confirm-grid" ]
                [ confirmField "Mobile" model.mobile
                , confirmField "Email" model.email
                ]
            ]

        -- Address
        , div [ class "confirm-section" ]
            [ div [ class "confirm-header" ]
                [ h3 [] [ text "Address" ]
                , button [ class "btn btn-sm btn-ghost", onClick (EditSection StepAddress) ] [ text "Edit" ]
                ]
            , div [ class "confirm-grid" ]
                [ confirmField "First name" a.firstName
                , confirmField "Last name" a.lastName
                , confirmField "Date of birth" a.dob
                , confirmField "Address line 1" a.line1
                , confirmField "Address line 2" (if String.isEmpty a.line2 then "—" else a.line2)
                , confirmField "City" a.city
                , confirmField "County / State" (if String.isEmpty a.county then "—" else a.county)
                , confirmField "Postcode" a.postcode
                , confirmField "Country" a.country
                ]
            ]

        -- Bank
        , div [ class "confirm-section" ]
            [ div [ class "confirm-header" ]
                [ h3 [] [ text "Bank Details" ]
                , button [ class "btn btn-sm btn-ghost", onClick (EditSection StepBank) ] [ text "Edit" ]
                ]
            , div [ class "confirm-grid" ]
                ([ confirmField "Account holder" b.holderName
                 , confirmField "Bank" b.bankName
                 , confirmField "Currency" b.currency
                 ]
                    ++ (if b.currency == "GBP" then
                            [ confirmField "Account number" b.accountNumber
                            , confirmField "Sort code" b.sortCode
                            ]
                        else
                            [ confirmField "IBAN" b.iban
                            , confirmField "SWIFT / BIC" b.swiftBic
                            ]
                       )
                )
            ]

        -- Documents
        , div [ class "confirm-section" ]
            [ div [ class "confirm-header" ]
                [ h3 [] [ text "Documents" ]
                , button [ class "btn btn-sm btn-ghost", onClick (EditSection StepDocuments) ] [ text "Edit" ]
                ]
            , ul [ class "confirm-doc-list" ]
                (List.map
                    (\d ->
                        li []
                            [ span [] [ text d.fileName ]
                            , span [ class "doc-badge" ]
                                [ text (Maybe.map docTypeLabel d.docType |> Maybe.withDefault "Unknown") ]
                            , span [ style "color" "#48bb78", style "font-size" "0.78rem" ] [ text "✓ Uploaded" ]
                            ]
                    )
                    model.documents
                )
            ]

        , div [ class "form-actions" ]
            [ button [ class "btn btn-primary", onClick SubmitOnboarding ]
                [ text "Submit Onboarding" ]
            ]
        ]


-- ── STEP: COMPLETE ────────────────────────────────────────────────────────────


viewComplete : Html Msg
viewComplete =
    div [ class "card complete-card" ]
        [ div [ class "complete-icon" ] [ text "✓" ]
        , h2 [] [ text "Onboarding Complete!" ]
        , p [] [ text "Your payee onboarding has been successfully submitted. You will receive a confirmation email shortly." ]
        ]


-- ── SHARED VIEW HELPERS ───────────────────────────────────────────────────────


formField : String -> Html Msg -> Html Msg
formField lbl input =
    div [ class "form-group" ]
        [ label [ class "form-label" ] [ text lbl ]
        , input
        ]


textInput : String -> String -> (String -> Msg) -> Html Msg
textInput ph val toMsg =
    input
        [ class "form-input"
        , type_ "text"
        , placeholder ph
        , value val
        , onInput toMsg
        ]
        []


viewErrors : List String -> Html Msg
viewErrors errors =
    if List.isEmpty errors then
        text ""
    else
        div [ class "error-box" ]
            (List.map (\e -> p [] [ text e ]) errors)


viewFieldError : Maybe String -> Html Msg
viewFieldError maybeErr =
    case maybeErr of
        Just err ->
            div [ class "form-error" ] [ text err ]

        Nothing ->
            text ""


viewGlobalError : Maybe String -> Html Msg
viewGlobalError maybeErr =
    case maybeErr of
        Just err ->
            div [ class "error-box" ] [ text err ]

        Nothing ->
            text ""


viewEditingBanner : Model -> Html Msg
viewEditingBanner model =
    case model.editingFrom of
        Just _ ->
            div [ class "editing-banner" ]
                [ text "✎ You are editing a completed section. Save to return to the review screen." ]

        Nothing ->
            text ""


confirmField : String -> String -> Html Msg
confirmField lbl val =
    div [ class "confirm-field" ]
        [ label [] [ text lbl ]
        , span [] [ text (if String.isEmpty val then "—" else val) ]
        ]


docTypeLabel : DocumentType -> String
docTypeLabel dt =
    case dt of
        Passport -> "Passport"
        BankStatement -> "Bank Statement"
        UtilityBill -> "Utility Bill"
        DrivingLicence -> "Driving Licence"
        OtherDoc -> "Other"


backButton : Step -> Model -> Html Msg
backButton target model =
    case model.editingFrom of
        Just _ ->
            text ""

        Nothing ->
            button
                [ class "btn btn-ghost"
                , onClick (NavigateToStep target)
                ]
                [ text "← Back" ]


saveLabel : String -> Model -> String
saveLabel default model =
    case model.editingFrom of
        Just _ ->
            "Save & Return to Review"

        Nothing ->
            default


onEnter : Msg -> Attribute Msg
onEnter msg =
    on "keydown"
        (Decode.field "key" Decode.string
            |> Decode.andThen
                (\key ->
                    if key == "Enter" then
                        Decode.succeed msg
                    else
                        Decode.fail "not Enter"
                )
        )


onFileChange : (String -> Msg) -> Attribute Msg
onFileChange toMsg =
    on "change"
        (Decode.at [ "target", "files", "0", "name" ] Decode.string
            |> Decode.map toMsg)


-- ── MAIN ──────────────────────────────────────────────────────────────────────


main : Program () Model Msg
main =
    Browser.element
        { init = init
        , update = update
        , view = view
        , subscriptions = \_ -> Sub.none
        }

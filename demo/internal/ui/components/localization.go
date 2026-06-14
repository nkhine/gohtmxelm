package components

import (
	"encoding/json"
	"time"

	"github.com/a-h/templ"

	"github.com/nkhine/gohtmxelm/demo/internal/localize"
	gohtmxelm "github.com/nkhine/gohtmxelm/pkg"
)

type LocalizationVM struct {
	Locale          localize.Locale
	Options         []localize.Option
	Title           string
	Subtitle        string
	SelectorLabel   string
	ServerHeading   string
	SampleSentence  string
	MoneyLabel      string
	MoneyValue      string
	DateLabel       string
	DateValue       string
	RelativeLabel   string
	RelativeValue   string
	LocaleLabel     string
	TimezoneLabel   string
	CurrencyLabel   string
	DatastarHeading string
	ElmHeading      string
	SignalsJSON     string
	ElmHTML         templ.Component
}

var localizationSampleTime = time.Date(2026, 6, 14, 13, 45, 0, 0, time.UTC)

func BuildLocalizationVM(store *localize.Store, tag string) LocalizationVM {
	loc := localize.Resolve(tag)
	tr := store.Translator(loc.Tag)
	money := localize.FormatMoney(loc, 1234567)
	posted := localize.FormatDate(loc, localizationSampleTime, tr.Text("common.date_format"))
	relative := tr.Text("localization.relative_value", map[string]any{"Count": 18})

	signals := map[string]any{
		"i18nHeading":   tr.Text("localization.datastar"),
		"moneyLabel":    tr.Text("localization.money_label"),
		"moneyValue":    money,
		"dateLabel":     tr.Text("localization.date_label"),
		"dateValue":     posted,
		"relativeLabel": tr.Text("localization.relative_label"),
		"relativeValue": relative,
	}
	signalsJSON, _ := json.Marshal(signals)

	props, _ := gohtmxelm.LocalizedProps(map[string]any{
		"title": tr.Text("localization.elm"),
		"sample": tr.Text("localization.sample_sentence", map[string]any{
			"Count": 3,
		}),
	}, gohtmxelm.LocalePropsFrom(loc.Tag, loc.Timezone, loc.Currency, tr,
		"localization.locale_label",
		"localization.timezone_label",
		"localization.currency_label",
	))
	elmHTML, _ := gohtmxelm.ElmIsland("locale-echo", "LocaleEcho", props)

	return LocalizationVM{
		Locale:          loc,
		Options:         store.Options(loc.Tag),
		Title:           tr.Text("localization.title"),
		Subtitle:        tr.Text("localization.subtitle"),
		SelectorLabel:   tr.Text("localization.selector"),
		ServerHeading:   tr.Text("localization.rendered_copy"),
		SampleSentence:  tr.Text("localization.sample_sentence", map[string]any{"Count": 3}),
		MoneyLabel:      tr.Text("localization.money_label"),
		MoneyValue:      money,
		DateLabel:       tr.Text("localization.date_label"),
		DateValue:       posted,
		RelativeLabel:   tr.Text("localization.relative_label"),
		RelativeValue:   relative,
		LocaleLabel:     tr.Text("localization.locale_label"),
		TimezoneLabel:   tr.Text("localization.timezone_label"),
		CurrencyLabel:   tr.Text("localization.currency_label"),
		DatastarHeading: tr.Text("localization.datastar"),
		ElmHeading:      tr.Text("localization.elm"),
		SignalsJSON:     string(signalsJSON),
		ElmHTML:         templ.Raw(elmHTML),
	}
}

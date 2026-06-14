package localize

import (
	"testing"
	"time"
)

func TestTranslatorTextAndMessages(t *testing.T) {
	store := MustStore()
	tr := store.Translator("fr-FR")

	got := tr.Text("localization.sample_sentence", map[string]any{"Count": 3})
	want := "Un réviseur de trésorerie a approuvé 3 virements."
	if got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}

	msgs := tr.Messages("localization.title", "missing.key")
	if msgs["localization.title"] != "Frontière de localisation" {
		t.Fatalf("localized title = %q", msgs["localization.title"])
	}
	if msgs["missing.key"] != "!missing.key!" {
		t.Fatalf("missing key = %q", msgs["missing.key"])
	}
}

func TestResolveFallsBack(t *testing.T) {
	if got := Resolve("nope").Tag; got != "en-GB" {
		t.Fatalf("Resolve fallback = %q, want en-GB", got)
	}
}

func TestFormatDateAndMoney(t *testing.T) {
	ts := time.Date(2026, 6, 14, 13, 45, 0, 0, time.UTC)
	fr := Resolve("fr-FR")

	if got := FormatDate(fr, ts, "02/01/2006 15:04"); got != "14/06/2026 15:45" {
		t.Fatalf("FormatDate = %q", got)
	}
	if got := FormatMoney(fr, 1234567); got != "12 345,67 €" {
		t.Fatalf("FormatMoney = %q", got)
	}
}

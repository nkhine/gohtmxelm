package gohtmxelm

import (
	"reflect"
	"testing"
)

type fakeCatalog map[string]string

func (f fakeCatalog) Messages(keys ...string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = f[key]
	}
	return out
}

func TestLocalePropsFromScopesMessages(t *testing.T) {
	props := LocalePropsFrom("fr-FR", "Europe/Paris", "EUR", fakeCatalog{
		"common.save":   "Enregistrer",
		"common.cancel": "Annuler",
	}, "common.save")

	if props.Locale != "fr-FR" || props.Timezone != "Europe/Paris" || props.Currency != "EUR" {
		t.Fatalf("unexpected locale props: %+v", props)
	}
	want := map[string]string{"common.save": "Enregistrer"}
	if !reflect.DeepEqual(props.Messages, want) {
		t.Fatalf("Messages = %#v, want %#v", props.Messages, want)
	}
}

func TestLocalizedPropsMergesStructBase(t *testing.T) {
	base := struct {
		Initial int    `json:"initial"`
		Name    string `json:"name"`
	}{Initial: 7, Name: "counter"}

	got, err := LocalizedProps(base, LocaleProps{
		Locale:   "en-GB",
		Timezone: "Europe/London",
		Currency: "GBP",
		Messages: map[string]string{"counter.title": "Counter"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for key, want := range map[string]any{
		"initial":  float64(7),
		"name":     "counter",
		"locale":   "en-GB",
		"timezone": "Europe/London",
		"currency": "GBP",
	} {
		if got[key] != want {
			t.Fatalf("%s = %#v, want %#v in %#v", key, got[key], want, got)
		}
	}
	if !reflect.DeepEqual(got["messages"], map[string]string{"counter.title": "Counter"}) {
		t.Fatalf("messages = %#v", got["messages"])
	}
}

func TestLocalizedPropsLocaleWinsOnCollision(t *testing.T) {
	got, err := LocalizedProps(map[string]any{
		"locale": "stale",
		"answer": 42,
	}, LocaleProps{Locale: "en-US"})
	if err != nil {
		t.Fatal(err)
	}
	if got["locale"] != "en-US" {
		t.Fatalf("locale = %#v, want en-US", got["locale"])
	}
	if got["answer"] != float64(42) {
		t.Fatalf("answer = %#v, want 42", got["answer"])
	}
}

func TestLocalizedPropsRejectsNonObjectBase(t *testing.T) {
	if _, err := LocalizedProps([]string{"not", "object"}, LocaleProps{Locale: "en-GB"}); err == nil {
		t.Fatal("expected error for non-object props")
	}
}

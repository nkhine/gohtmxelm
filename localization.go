package gohtmxelm

import (
	"encoding/json"
	"fmt"
)

// MessageCatalog is the minimal surface an application translator needs to
// expose when it wants to pass a scoped message bundle into an Elm island.
// The library does not own catalogue loading, fallback, interpolation, or
// pluralisation policy; host applications provide that behind this shape.
type MessageCatalog interface {
	Messages(keys ...string) map[string]string
}

// LocaleProps is the neutral localization payload convention used by Elm
// island flags. Locale is normally a BCP-47 tag (for example "en-GB"),
// Timezone an IANA identifier, Currency an ISO-4217 alpha code, and Messages a
// small key/value bundle scoped to the island.
type LocaleProps struct {
	Locale   string            `json:"locale,omitempty"`
	Timezone string            `json:"timezone,omitempty"`
	Currency string            `json:"currency,omitempty"`
	Messages map[string]string `json:"messages,omitempty"`
}

// LocalePropsFrom builds LocaleProps from a host application's message
// catalogue. It deliberately treats a nil catalogue as valid so pages can
// still mount islands with locale metadata while server-rendered views own all
// copy.
func LocalePropsFrom(locale, timezone, currency string, catalog MessageCatalog, keys ...string) LocaleProps {
	props := LocaleProps{
		Locale:   locale,
		Timezone: timezone,
		Currency: currency,
	}
	if catalog != nil && len(keys) > 0 {
		props.Messages = catalog.Messages(keys...)
	}
	return props
}

// LocalizedProps merges application-specific props with the standard locale
// payload. The returned map is suitable for ElmIsland's props argument.
//
// The base value must encode as a JSON object (struct or map). Locale fields
// win on key collisions because callers use this helper when they want a
// canonical localization payload.
func LocalizedProps(base any, locale LocaleProps) (map[string]any, error) {
	out := map[string]any{}
	if base != nil {
		m, err := objectMap(base)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			out[k] = v
		}
	}
	if locale.Locale != "" {
		out["locale"] = locale.Locale
	}
	if locale.Timezone != "" {
		out["timezone"] = locale.Timezone
	}
	if locale.Currency != "" {
		out["currency"] = locale.Currency
	}
	if len(locale.Messages) > 0 {
		out["messages"] = locale.Messages
	}
	return out, nil
}

func objectMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("gohtmxelm: marshal props: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("gohtmxelm: props must encode as a JSON object: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("gohtmxelm: props must encode as a JSON object")
	}
	return m, nil
}

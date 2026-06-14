# Localization Boundary

`gohtmxelm` supports i18n/l10n as an integration convention, not as a framework
policy.

The library should carry locale data across HTMX, Datastar, Elm, and SSE. It
should not decide which locales exist, how a user preference is resolved, how
catalogues are loaded, or how money and dates are formatted. Those choices are
application policy.

## Ownership

| Layer | Owns |
|---|---|
| Application | Supported locales, catalogue files, fallback rules, cookies/session/user preferences, interpolation, pluralisation, date/money formatting. |
| `gohtmxelm` | A neutral props convention for passing locale metadata and scoped messages to Elm islands. |
| HTMX | Receives already-localized server-rendered fragments. |
| Datastar | Receives already-localized DOM patches or signal values. |
| Elm | Receives locale metadata and a small message bundle in flags; richer copy should usually be re-rendered by Go. |

## Elm Props Convention

Use `LocalePropsFrom` and `LocalizedProps` to build flags:

```go
locale := gohtmxelm.LocalePropsFrom("fr-FR", "Europe/Paris", "EUR", translator,
	"common.save",
	"invoice.title",
)
props, err := gohtmxelm.LocalizedProps(domainProps, locale)
if err != nil {
	return err
}
html, err := gohtmxelm.ElmIsland("invoice-editor", "InvoiceEditor", props)
```

The resulting JSON shape is:

```json
{
  "locale": "fr-FR",
  "timezone": "Europe/Paris",
  "currency": "EUR",
  "messages": {
    "common.save": "Enregistrer"
  }
}
```

The `translator` only needs this structural method:

```go
Messages(keys ...string) map[string]string
```

That keeps the reusable package independent from TOML, gettext, ICU Message
Format, database-backed catalogues, or any future translation service.

## Locale Switch Flow

A typical locale switch stays server-led:

1. The user chooses a locale in an HTMX form.
2. The app validates the tag and persists it to its cookie/session/profile.
3. The app rebuilds its request view model under the new translator/formatter.
4. HTMX swaps the localized fragments.
5. Datastar gets new localized signal values or patches.
6. Elm islands that were replaced remount with fresh flags. Long-lived islands
   can also listen for an app-defined locale-changed event and refetch or merge
   a scoped message delta.

The demo card at `/examples/localization` shows this boundary using a
demo-owned TOML-style catalogue in `demo/internal/localize`.

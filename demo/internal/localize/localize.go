package localize

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

//go:embed locales.toml
var localeFS embed.FS

// Locale is the demo application's locale policy. gohtmxelm only receives the
// resulting tag/timezone/currency as props; this registry deliberately lives in
// the host app.
type Locale struct {
	Tag      string
	Timezone string
	Currency string
}

type Option struct {
	Locale Locale
	Label  string
}

type Store struct {
	catalogues map[string]map[string]string
}

type Translator struct {
	messages map[string]string
}

var supported = []Locale{
	{Tag: "en-GB", Timezone: "Europe/London", Currency: "GBP"},
	{Tag: "en-US", Timezone: "America/New_York", Currency: "USD"},
	{Tag: "fr-FR", Timezone: "Europe/Paris", Currency: "EUR"},
}

var DefaultLocale = supported[0]

func NewStore() (*Store, error) {
	b, err := localeFS.ReadFile("locales.toml")
	if err != nil {
		return nil, err
	}
	cats, err := parseCatalogue(string(b))
	if err != nil {
		return nil, err
	}
	return &Store{catalogues: cats}, nil
}

func MustStore() *Store {
	s, err := NewStore()
	if err != nil {
		panic(err)
	}
	return s
}

func Supported() []Locale {
	out := make([]Locale, len(supported))
	copy(out, supported)
	return out
}

func Resolve(tag string) Locale {
	for _, loc := range supported {
		if loc.Tag == tag {
			return loc
		}
	}
	return DefaultLocale
}

func (s *Store) Translator(tag string) Translator {
	loc := Resolve(tag)
	cat := map[string]string{}
	if base, ok := s.catalogues["en-GB"]; ok {
		for k, v := range base {
			cat[k] = v
		}
	}
	if regional, ok := s.catalogues[loc.Tag]; ok {
		for k, v := range regional {
			cat[k] = v
		}
	}
	return Translator{messages: cat}
}

func (s *Store) Options(current string) []Option {
	options := make([]Option, 0, len(supported))
	for _, loc := range supported {
		t := s.Translator(loc.Tag)
		options = append(options, Option{Locale: loc, Label: t.Text("locale.name")})
	}
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].Locale.Tag == current {
			return true
		}
		if options[j].Locale.Tag == current {
			return false
		}
		return options[i].Label < options[j].Label
	})
	return options
}

func (t Translator) Text(key string, args ...any) string {
	raw, ok := t.messages[key]
	if !ok {
		return "!" + key + "!"
	}
	if len(args) == 0 || !strings.Contains(raw, "{{") {
		return raw
	}
	tpl, err := template.New(key).Option("missingkey=zero").Parse(raw)
	if err != nil {
		return "!" + key + ":template-error!"
	}
	data := any(args[0])
	if len(args) > 1 {
		data = args
	}
	var b strings.Builder
	if err := tpl.Execute(&b, data); err != nil {
		return "!" + key + ":render-error!"
	}
	return b.String()
}

func (t Translator) Messages(keys ...string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = t.Text(key)
	}
	return out
}

func FormatDate(loc Locale, ts time.Time, layout string) string {
	zone, err := time.LoadLocation(loc.Timezone)
	if err != nil {
		zone = time.UTC
	}
	if layout == "" || strings.HasPrefix(layout, "!") {
		layout = "02 Jan 2006 15:04"
	}
	return ts.In(zone).Format(layout)
}

func FormatMoney(loc Locale, minor int64) string {
	whole := minor / 100
	cents := minor % 100
	if cents < 0 {
		cents = -cents
	}
	switch loc.Currency {
	case "USD":
		return "$" + comma(whole) + fmt.Sprintf(".%02d", cents)
	case "EUR":
		return strings.ReplaceAll(comma(whole), ",", " ") + fmt.Sprintf(",%02d €", cents)
	default:
		return "£" + comma(whole) + fmt.Sprintf(".%02d", cents)
	}
}

func parseCatalogue(src string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	section := ""
	for lineNo, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if section == "" {
				return nil, fmt.Errorf("line %d: empty section", lineNo+1)
			}
			if out[section] == nil {
				out[section] = map[string]string{}
			}
			continue
		}
		if section == "" {
			return nil, fmt.Errorf("line %d: key outside section", lineNo+1)
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected key = value", lineNo+1)
		}
		key = strings.Trim(strings.TrimSpace(key), `"`)
		parsed, err := strconv.Unquote(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("line %d: parse value: %w", lineNo+1, err)
		}
		out[section][key] = parsed
	}
	return out, nil
}

func comma(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return sign + s
	}
	var b strings.Builder
	b.WriteString(sign)
	prefix := len(s) % 3
	if prefix == 0 {
		prefix = 3
	}
	b.WriteString(s[:prefix])
	for i := prefix; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

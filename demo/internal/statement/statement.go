// Package statement is the demo's bank-statement domain: a GBP account whose
// funds transfers are server-owned mock data (shaped after a generic treasury
// payment rows), and a server-selected date range that every front-end surface
// observes over SSE. Go owns the truth — the seeded transfers and the active
// range; HTMX renders the statement table, Datastar renders the live summary,
// and the Elm range picker drives the selection.
package statement

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	gohtmxelm "github.com/nkhine/gohtmxelm/pkg"
)

// dateTimeLocal is the layout produced by an <input type="datetime-local">.
const dateTimeLocal = "2006-01-02T15:04"

// Transfer is one funds movement on the statement. The shape mirrors a generic
// treasury payment row (direction DEBIT/CREDIT, amount, currency, reference,
// method, status, name on account, end-to-end id).
type Transfer struct {
	ID           string    `json:"id"`
	At           time.Time `json:"at"`
	EndToEndID   string    `json:"endToEndId"`
	Counterparty string    `json:"counterparty"`
	Reference    string    `json:"reference"`
	Method       string    `json:"method"`    // FPS | BACS | CHAPS | SWIFT | SEPA
	Direction    string    `json:"direction"` // DEBIT | CREDIT
	AmountMinor  int64     `json:"amountMinor"`
	Currency     string    `json:"currency"` // always GBP for this single-currency account
	Status       string    `json:"status"`   // SETTLED | PENDING | RETURNED
	IsRefund     bool      `json:"isRefund"`
}

// SignedMinor is the amount as it affects the balance: credits add, debits
// subtract.
func (t Transfer) SignedMinor() int64 {
	if t.Direction == "CREDIT" {
		return t.AmountMinor
	}
	return -t.AmountMinor
}

// Range is a half-open-ish [From, To] window (inclusive of both ends here,
// which is fine for a demo's millisecond-free timestamps).
type Range struct {
	From  time.Time
	To    time.Time
	Label string
}

// Summary is the aggregate view of the transfers inside a range, plus the
// opening balance carried in from everything before the window.
type Summary struct {
	Count        int   `json:"count"`
	CreditsMinor int64 `json:"creditsMinor"`
	DebitsMinor  int64 `json:"debitsMinor"`
	OpeningMinor int64 `json:"openingMinor"`
	ClosingMinor int64 `json:"closingMinor"`
}

// RangeEvent is published whenever the active range changes.
type RangeEvent struct {
	Range   Range
	Summary Summary
}

// presets maps a preset key to its window duration, newest-relative.
var presets = []struct {
	Key      string
	Label    string
	Duration time.Duration
}{
	{"15m", "Last 15 min", 15 * time.Minute},
	{"30m", "Last 30 min", 30 * time.Minute},
	{"1h", "Last 1 hour", time.Hour},
	{"3h", "Last 3 hours", 3 * time.Hour},
	{"24h", "Last 24 hours", 24 * time.Hour},
	{"2d", "Last 2 days", 48 * time.Hour},
	{"2w", "Last 2 weeks", 14 * 24 * time.Hour},
	{"1mo", "Last 1 month", 30 * 24 * time.Hour},
	{"3mo", "Last 3 months", 90 * 24 * time.Hour},
}

func presetByKey(key string) (time.Duration, string, bool) {
	for _, p := range presets {
		if p.Key == key {
			return p.Duration, p.Label, true
		}
	}
	return 0, "", false
}

// Presets exposes the (key, label) pairs for rendering the picker server-side
// if desired; the Elm picker carries its own copy.
func Presets() []struct {
	Key   string
	Label string
} {
	out := make([]struct {
		Key   string
		Label string
	}, 0, len(presets))
	for _, p := range presets {
		out = append(out, struct {
			Key   string
			Label string
		}{p.Key, p.Label})
	}
	return out
}

// Statement holds the seeded transfers (immutable after construction) and the
// currently-selected range, with change notifications.
type Statement struct {
	mu        sync.RWMutex
	transfers []Transfer // sorted ascending by At
	rng       Range
	events    *gohtmxelm.Broadcaster[RangeEvent]
	now       func() time.Time
}

// New seeds a statement relative to the supplied clock's now and selects a
// default range (last 24 hours).
func New(now func() time.Time) *Statement {
	if now == nil {
		now = time.Now
	}
	s := &Statement{
		transfers: seed(now()),
		events:    gohtmxelm.NewBroadcaster[RangeEvent](16),
		now:       now,
	}
	d, label, _ := presetByKey("24h")
	t := now()
	s.rng = Range{From: t.Add(-d), To: t, Label: label}
	return s
}

// Events exposes the broadcaster for SSE handlers.
func (s *Statement) Events() *gohtmxelm.Broadcaster[RangeEvent] { return s.events }

// Range returns the active range.
func (s *Statement) Range() Range {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rng
}

// ApplyPreset selects a preset window ending at now.
func (s *Statement) ApplyPreset(key string) (Range, error) {
	d, label, ok := presetByKey(key)
	if !ok {
		return Range{}, fmt.Errorf("unknown preset %q", key)
	}
	now := s.now()
	return s.setRange(Range{From: now.Add(-d), To: now, Label: label}), nil
}

// ApplyCustom selects a custom window from two datetime-local strings.
func (s *Statement) ApplyCustom(fromStr, toStr string) (Range, error) {
	from, err := time.ParseInLocation(dateTimeLocal, strings.TrimSpace(fromStr), time.Local)
	if err != nil {
		return Range{}, errors.New("invalid 'from' datetime")
	}
	to, err := time.ParseInLocation(dateTimeLocal, strings.TrimSpace(toStr), time.Local)
	if err != nil {
		return Range{}, errors.New("invalid 'to' datetime")
	}
	if to.Before(from) {
		return Range{}, errors.New("'to' is before 'from'")
	}
	return s.setRange(Range{From: from, To: to, Label: "Custom range"}), nil
}

func (s *Statement) setRange(r Range) Range {
	s.mu.Lock()
	s.rng = r
	s.mu.Unlock()
	s.events.Publish(RangeEvent{Range: r, Summary: s.Summary(r)})
	return r
}

// Transfers returns the transfers inside r, newest first.
func (s *Statement) Transfers(r Range) []Transfer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Transfer
	for _, t := range s.transfers {
		if !t.At.Before(r.From) && !t.At.After(r.To) {
			out = append(out, t)
		}
	}
	// transfers are stored ascending; reverse for newest-first display.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Summary aggregates the transfers inside r and the opening balance carried in
// from everything strictly before r.From.
func (s *Statement) Summary(r Range) Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var sum Summary
	for _, t := range s.transfers {
		if t.At.Before(r.From) {
			sum.OpeningMinor += t.SignedMinor()
			continue
		}
		if t.At.After(r.To) {
			continue
		}
		sum.Count++
		if t.Direction == "CREDIT" {
			sum.CreditsMinor += t.AmountMinor
		} else {
			sum.DebitsMinor += t.AmountMinor
		}
	}
	sum.ClosingMinor = sum.OpeningMinor + sum.CreditsMinor - sum.DebitsMinor
	return sum
}

// RunningBalance returns the closing balance after each transfer in the given
// newest-first slice, keyed by transfer ID, computed from the opening balance.
// It lets the table show a per-row balance without recomputing in the view.
func RunningBalance(opening int64, newestFirst []Transfer) map[string]int64 {
	balances := make(map[string]int64, len(newestFirst))
	// Walk oldest-first to accumulate.
	running := opening
	for i := len(newestFirst) - 1; i >= 0; i-- {
		t := newestFirst[i]
		running += t.SignedMinor()
		balances[t.ID] = running
	}
	return balances
}

// FormatGBP formats minor units as a £ amount with thousands separators.
func FormatGBP(minor int64) string {
	neg := minor < 0
	if neg {
		minor = -minor
	}
	pounds := minor / 100
	pence := minor % 100
	out := fmt.Sprintf("£%s.%02d", withThousands(pounds), pence)
	if neg {
		return "-" + out
	}
	return out
}

func withThousands(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if len(s) > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}

// seed builds a deterministic set of GBP transfers relative to now: a few in
// the last 15 minutes (so the tightest presets always show data) plus a spread
// across the last 90 days.
func seed(now time.Time) []Transfer {
	rng := rand.New(rand.NewSource(1))

	counterparties := []string{
		"Northwind Traders", "Acme Logistics", "Globex Corp", "Initech Ltd",
		"Umbrella Health", "Stark Industries", "Wayne Enterprises", "Soylent Foods",
		"Hooli Cloud", "Pied Piper", "Wonka Confectionery", "Cyberdyne Systems",
		"Gringotts Bank", "Tyrell Refunds", "Massive Dynamic",
	}
	methods := []string{"FPS", "BACS", "CHAPS", "SWIFT", "SEPA"}
	statuses := []struct {
		v string
		w int
	}{{"SETTLED", 80}, {"PENDING", 14}, {"RETURNED", 6}}

	pickStatus := func() string {
		n := rng.Intn(100)
		acc := 0
		for _, s := range statuses {
			acc += s.w
			if n < acc {
				return s.v
			}
		}
		return "SETTLED"
	}

	// Offsets (from now) guaranteeing recent coverage, then a long spread.
	var offsets []time.Duration
	for _, m := range []int{1, 4, 7, 11, 14} { // within last 15 min
		offsets = append(offsets, time.Duration(m)*time.Minute)
	}
	for i := 0; i < 235; i++ { // 15 min .. 90 days
		mins := 15 + rng.Intn(90*24*60)
		offsets = append(offsets, time.Duration(mins)*time.Minute)
	}

	out := make([]Transfer, 0, len(offsets))
	for i, off := range offsets {
		dir := "DEBIT"
		if rng.Intn(2) == 0 {
			dir = "CREDIT"
		}
		amount := int64(1000 + rng.Intn(499000)) // £10.00 .. £5,000.00
		isRefund := rng.Intn(40) == 0
		ref := fmt.Sprintf("INV-%05d", 10000+rng.Intn(89999))
		if isRefund {
			ref = "REFUND-" + ref
			dir = "CREDIT"
		}
		out = append(out, Transfer{
			ID:           fmt.Sprintf("TR-%05d", i+1),
			At:           now.Add(-off),
			EndToEndID:   fmt.Sprintf("E2E%08x", rng.Uint32()),
			Counterparty: counterparties[rng.Intn(len(counterparties))],
			Reference:    ref,
			Method:       methods[rng.Intn(len(methods))],
			Direction:    dir,
			AmountMinor:  amount,
			Currency:     "GBP",
			Status:       pickStatus(),
			IsRefund:     isRefund,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

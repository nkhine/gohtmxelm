// Package statement is the demo's bank-statement domain: a GBP account whose
// funds transfers live in an in-memory DynamoDB-style table, and a
// server-selected date range that every front-end surface observes over SSE.
// The transfers are not hard-coded — they are faked into the table at runtime
// by the Seed example card. Go owns the truth; HTMX renders the statement
// table, Datastar renders the live summary, and the Elm range picker drives the
// selection.
package statement

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/nkhine/gohtmxelm"
	"github.com/nkhine/gohtmxelm/demo/internal/dynamo"
)

// dateTimeLocal is the layout produced by an <input type="datetime-local">.
const dateTimeLocal = "2006-01-02T15:04"

// Transfer is one funds movement on the statement. The shape mirrors heritage's
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
	Currency     string    `json:"currency"`
	Status       string    `json:"status"` // SETTLED | PENDING | RETURNED
	IsRefund     bool      `json:"isRefund"`
}

// Transfer status values, aligned with the unimatrix ledger: POSTED and
// PENDING contribute to the balance buckets; RETURNED is a failed/reversed
// movement that appears on the statement but contributes to no balance.
const (
	StatusPosted   = "POSTED"
	StatusPending  = "PENDING"
	StatusReturned = "RETURNED"
)

// item renders the transfer as a DynamoDB-style attribute map. "id" is the
// partition key; "atUnix" is the sort attribute.
func (t Transfer) item() dynamo.Item {
	return dynamo.Item{
		"id":           t.ID,
		"endToEndId":   t.EndToEndID,
		"counterparty": t.Counterparty,
		"reference":    t.Reference,
		"method":       t.Method,
		"direction":    t.Direction,
		"amountMinor":  t.AmountMinor,
		"currency":     t.Currency,
		"status":       t.Status,
		"isRefund":     t.IsRefund,
		"atUnix":       t.At.Unix(),
	}
}

func transferFromItem(it dynamo.Item) Transfer {
	return Transfer{
		ID:           itemStr(it, "id"),
		At:           time.Unix(dynamo.AsInt64(it["atUnix"]), 0),
		EndToEndID:   itemStr(it, "endToEndId"),
		Counterparty: itemStr(it, "counterparty"),
		Reference:    itemStr(it, "reference"),
		Method:       itemStr(it, "method"),
		Direction:    itemStr(it, "direction"),
		AmountMinor:  dynamo.AsInt64(it["amountMinor"]),
		Currency:     itemStr(it, "currency"),
		Status:       itemStr(it, "status"),
		IsRefund:     itemBool(it, "isRefund"),
	}
}

func itemStr(it dynamo.Item, k string) string { s, _ := it[k].(string); return s }
func itemBool(it dynamo.Item, k string) bool  { b, _ := it[k].(bool); return b }

// Range is an inclusive [From, To] window with a human-readable label.
type Range struct {
	From  time.Time
	To    time.Time
	Label string
}

// Summary is the unimatrix-style four-bucket view of the transfers inside a
// range: posted vs pending, credit vs debit. There is no synthetic "opening
// balance" — real balances are derived from these buckets.
type Summary struct {
	Count          int   `json:"count"`
	CreditsPosted  int64 `json:"creditsPosted"`
	DebitsPosted   int64 `json:"debitsPosted"`
	CreditsPending int64 `json:"creditsPending"`
	DebitsPending  int64 `json:"debitsPending"`
}

// AvailableMinor is the unimatrix available balance for the window:
// credits_posted - debits_posted - debits_pending, clamped at zero (pending
// credits do not count toward spendable funds; pending debits reserve them).
func (sum Summary) AvailableMinor() int64 {
	a := sum.CreditsPosted - sum.DebitsPosted - sum.DebitsPending
	if a < 0 {
		return 0
	}
	return a
}

// PostedNetMinor is the cleared movement over the window:
// credits_posted - debits_posted (signed).
func (sum Summary) PostedNetMinor() int64 {
	return sum.CreditsPosted - sum.DebitsPosted
}

// RangeEvent is published whenever the active range changes or new transfers
// are seeded.
type RangeEvent struct {
	Range   Range
	Summary Summary
}

var (
	methods    = []string{"FPS", "BACS", "CHAPS", "SWIFT", "SEPA"}
	currencies = []string{"GBP", "GBP", "GBP", "EUR", "USD"} // weighted toward GBP
	// Weighted ~70% posted, ~20% pending, ~10% returned.
	statuses = []string{
		StatusPosted, StatusPosted, StatusPosted, StatusPosted, StatusPosted,
		StatusPosted, StatusPosted, StatusPending, StatusPending, StatusReturned,
	}
)

// relativeRange resolves a "last N <unit>" window ending at now. unit is one of
// minutes, hours, days, weeks.
func relativeRange(now time.Time, value int, unit string) (Range, error) {
	// "all" is the list-everything filter: a window wide enough to cover every
	// seeded transfer (value is ignored).
	if unit == "all" {
		return Range{From: now.AddDate(-100, 0, 0), To: now, Label: "All transfers"}, nil
	}
	if value <= 0 {
		return Range{}, errors.New("value must be positive")
	}
	var (
		d    time.Duration
		noun string
	)
	switch unit {
	case "minutes":
		d, noun = time.Duration(value)*time.Minute, "minute"
	case "hours":
		d, noun = time.Duration(value)*time.Hour, "hour"
	case "days":
		d, noun = time.Duration(value)*24*time.Hour, "day"
	case "weeks":
		d, noun = time.Duration(value)*7*24*time.Hour, "week"
	default:
		return Range{}, fmt.Errorf("unknown unit %q", unit)
	}
	if value != 1 {
		noun += "s"
	}
	return Range{From: now.Add(-d), To: now, Label: fmt.Sprintf("Last %d %s", value, noun)}, nil
}

// Statement owns the in-memory transfer table and the currently-selected range.
type Statement struct {
	mu     sync.RWMutex // guards rng
	genMu  sync.Mutex   // guards the (non-thread-safe) faker
	table  *dynamo.Table
	faker  *gofakeit.Faker
	rng    Range
	events *gohtmxelm.Broadcaster[RangeEvent]
	now    func() time.Time
}

// New creates a statement backed by an empty in-memory DynamoDB-style table and
// a default range of the last day. It holds no transfers until the Seed card
// adds them — so the table count is exactly what has been seeded, nothing
// hidden. The faker uses a fixed seed for reproducible demo data.
func New(now func() time.Time) *Statement {
	if now == nil {
		now = time.Now
	}
	s := &Statement{
		table:  dynamo.NewDB().CreateTableIfNotExists("transfers", "id", "atUnix"),
		faker:  gofakeit.New(1),
		events: gohtmxelm.NewBroadcaster[RangeEvent](16),
		now:    now,
	}
	s.rng, _ = relativeRange(now(), 1, "days")
	return s
}

// Events exposes the broadcaster for SSE handlers.
func (s *Statement) Events() *gohtmxelm.Broadcaster[RangeEvent] { return s.events }

// Count returns the total number of transfers in the table.
func (s *Statement) Count() int { return s.table.Count() }

// Range returns the active range.
func (s *Statement) Range() Range {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rng
}

// Generate fakes count transfers whose timestamps are scattered uniformly
// across [now-period, now]. Counterparty names come from gofakeit.
func (s *Statement) Generate(count int, period time.Duration, now time.Time) []Transfer {
	if count < 0 {
		count = 0
	}
	spanSecs := int(period / time.Second)
	if spanSecs < 1 {
		spanSecs = 1
	}
	s.genMu.Lock()
	defer s.genMu.Unlock()
	f := s.faker

	out := make([]Transfer, 0, count)
	for i := 0; i < count; i++ {
		// Bias toward credits so the account stays realistically in credit.
		direction := "DEBIT"
		if f.Number(0, 99) < 58 {
			direction = "CREDIT"
		}
		isRefund := f.Number(0, 39) == 0
		ref := fmt.Sprintf("INV-%05d", f.Number(10000, 99999))
		if isRefund {
			direction = "CREDIT"
			ref = "REFUND-" + ref
		}
		out = append(out, Transfer{
			ID:           f.UUID(),
			At:           now.Add(-time.Duration(f.Number(0, spanSecs)) * time.Second),
			EndToEndID:   "E2E-" + f.LetterN(10),
			Counterparty: f.Company(),
			Reference:    ref,
			Method:       f.RandomString(methods),
			Direction:    direction,
			AmountMinor:  int64(f.Number(1000, 500000)), // £10.00 .. £5,000.00
			Currency:     f.RandomString(currencies),
			Status:       f.RandomString(statuses),
			IsRefund:     isRefund,
		})
	}
	return out
}

func (s *Statement) put(transfers []Transfer) {
	for _, t := range transfers {
		_ = s.table.PutItem(t.item())
	}
}

// Seed fakes count transfers across the given period, writes them into the
// table, and publishes the current range so every surface re-renders with the
// new data. Returns how many were created.
func (s *Statement) Seed(count int, period time.Duration) int {
	transfers := s.Generate(count, period, s.now())
	s.put(transfers)
	s.Touch()
	return len(transfers)
}

// Add writes a single transfer into the table without publishing. Pair it with
// Touch when streaming items one at a time (e.g. the Seed card's live feed).
func (s *Statement) Add(t Transfer) { _ = s.table.PutItem(t.item()) }

// Touch republishes the current range so subscribers re-render.
func (s *Statement) Touch() {
	r := s.Range()
	s.events.Publish(RangeEvent{Range: r, Summary: s.Summary(r)})
}

// ApplyRelative selects a "last N <unit>" window ending at now.
func (s *Statement) ApplyRelative(value int, unit string) (Range, error) {
	r, err := relativeRange(s.now(), value, unit)
	if err != nil {
		return Range{}, err
	}
	return s.setRange(r), nil
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
	out := make([]Transfer, 0)
	for _, it := range s.table.Scan() {
		t := transferFromItem(it)
		if !t.At.Before(r.From) && !t.At.After(r.To) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// Summary aggregates the transfers inside r into the four unimatrix buckets.
// RETURNED transfers are counted as rows but contribute to no bucket.
func (s *Statement) Summary(r Range) Summary {
	var sum Summary
	for _, it := range s.table.Scan() {
		t := transferFromItem(it)
		if t.At.Before(r.From) || t.At.After(r.To) {
			continue
		}
		sum.Count++
		credit := t.Direction == "CREDIT"
		switch t.Status {
		case StatusPosted:
			if credit {
				sum.CreditsPosted += t.AmountMinor
			} else {
				sum.DebitsPosted += t.AmountMinor
			}
		case StatusPending:
			if credit {
				sum.CreditsPending += t.AmountMinor
			} else {
				sum.DebitsPending += t.AmountMinor
			}
		}
	}
	return sum
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

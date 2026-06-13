package statement

import (
	"testing"
	"time"
)

// fixedNow returns a stable clock so seeded offsets and ranges are deterministic.
func fixedNow() time.Time {
	return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
}

func newFixed() *Statement {
	return New(fixedNow)
}

func TestDefaultRangeIsOneDay(t *testing.T) {
	s := newFixed()
	r := s.Range()
	if got := r.To.Sub(r.From); got != 24*time.Hour {
		t.Fatalf("default window = %v, want 24h", got)
	}
	if r.Label != "Last 1 day" {
		t.Fatalf("default label = %q", r.Label)
	}
}

func TestApplyRelativeResolvesWindowAndLabel(t *testing.T) {
	cases := []struct {
		value int
		unit  string
		want  time.Duration
		label string
	}{
		{15, "minutes", 15 * time.Minute, "Last 15 minutes"},
		{1, "hours", time.Hour, "Last 1 hour"},
		{12, "hours", 12 * time.Hour, "Last 12 hours"},
		{6, "days", 6 * 24 * time.Hour, "Last 6 days"},
		{2, "weeks", 14 * 24 * time.Hour, "Last 2 weeks"},
	}
	for _, c := range cases {
		s := newFixed()
		r, err := s.ApplyRelative(c.value, c.unit)
		if err != nil {
			t.Fatalf("%d %s: %v", c.value, c.unit, err)
		}
		if got := r.To.Sub(r.From); got != c.want {
			t.Fatalf("%d %s window = %v, want %v", c.value, c.unit, got, c.want)
		}
		if r.To != fixedNow() {
			t.Fatalf("relative range should end at now, got %v", r.To)
		}
		if r.Label != c.label {
			t.Fatalf("label = %q, want %q", r.Label, c.label)
		}
	}
}

func TestApplyRelativeRejectsBadInput(t *testing.T) {
	if _, err := newFixed().ApplyRelative(1, "fortnights"); err == nil {
		t.Fatal("expected error for unknown unit")
	}
	if _, err := newFixed().ApplyRelative(0, "hours"); err == nil {
		t.Fatal("expected error for non-positive value")
	}
}

func TestApplyCustomValidatesOrder(t *testing.T) {
	s := newFixed()
	if _, err := s.ApplyCustom("2025-06-01T10:00", "2025-06-01T09:00"); err == nil {
		t.Fatal("expected error when to is before from")
	}
	if _, err := s.ApplyCustom("not-a-date", "2025-06-01T09:00"); err == nil {
		t.Fatal("expected error for unparseable from")
	}
	if _, err := s.ApplyCustom("2025-06-01T09:00", "2025-06-01T10:00"); err != nil {
		t.Fatalf("valid custom range should succeed: %v", err)
	}
}

func TestSeedCoversTightPresets(t *testing.T) {
	s := newFixed()
	r, _ := s.ApplyRelative(15, "minutes")
	if len(s.Transfers(r)) == 0 {
		t.Fatal("expected at least one transfer in the last 15 minutes")
	}
}

func TestWiderPresetIncludesMoreOrEqual(t *testing.T) {
	s := newFixed()
	r15, _ := s.ApplyRelative(15, "minutes")
	r3mo, _ := s.ApplyRelative(12, "weeks")
	if len(s.Transfers(r3mo)) < len(s.Transfers(r15)) {
		t.Fatal("3-month window must include at least as many transfers as 15-min")
	}
}

func TestSummaryBalancesAreConsistent(t *testing.T) {
	s := newFixed()
	r, _ := s.ApplyRelative(12, "weeks")
	sum := s.Summary(r)
	if sum.ClosingMinor != sum.OpeningMinor+sum.CreditsMinor-sum.DebitsMinor {
		t.Fatalf("closing != opening + credits - debits: %+v", sum)
	}
	if sum.Count != len(s.Transfers(r)) {
		t.Fatalf("summary count %d != transfers %d", sum.Count, len(s.Transfers(r)))
	}
}

func TestRunningBalanceEndsAtClosing(t *testing.T) {
	s := newFixed()
	r, _ := s.ApplyRelative(12, "weeks")
	transfers := s.Transfers(r) // newest-first
	sum := s.Summary(r)
	balances := RunningBalance(sum.OpeningMinor, transfers)
	if len(transfers) == 0 {
		t.Skip("no transfers in window")
	}
	// The newest transfer (index 0) carries the closing balance.
	if balances[transfers[0].ID] != sum.ClosingMinor {
		t.Fatalf("running balance at newest = %d, want closing %d", balances[transfers[0].ID], sum.ClosingMinor)
	}
}

func TestFormatGBP(t *testing.T) {
	cases := map[int64]string{
		0:         "£0.00",
		5:         "£0.05",
		100:       "£1.00",
		123456:    "£1,234.56",
		-250000:   "-£2,500.00",
		100000000: "£1,000,000.00",
	}
	for minor, want := range cases {
		if got := FormatGBP(minor); got != want {
			t.Errorf("FormatGBP(%d) = %q, want %q", minor, got, want)
		}
	}
}

func TestRangeChangePublishes(t *testing.T) {
	s := newFixed()
	ch := s.Events().Subscribe()
	defer s.Events().Unsubscribe(ch)

	if _, err := s.ApplyRelative(1, "hours"); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-ch:
		if ev.Range.Label != "Last 1 hour" {
			t.Fatalf("unexpected range event %+v", ev.Range)
		}
		if ev.Summary.Count != len(s.Transfers(ev.Range)) {
			t.Fatal("event summary count mismatch")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for range event")
	}
}

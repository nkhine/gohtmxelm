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

func TestDefaultRangeIs24h(t *testing.T) {
	s := newFixed()
	r := s.Range()
	if got := r.To.Sub(r.From); got != 24*time.Hour {
		t.Fatalf("default window = %v, want 24h", got)
	}
	if r.Label != "Last 24 hours" {
		t.Fatalf("default label = %q", r.Label)
	}
}

func TestApplyPresetResolvesWindow(t *testing.T) {
	s := newFixed()
	r, err := s.ApplyPreset("15m")
	if err != nil {
		t.Fatal(err)
	}
	if got := r.To.Sub(r.From); got != 15*time.Minute {
		t.Fatalf("15m window = %v", got)
	}
	if r.To != fixedNow() {
		t.Fatalf("preset should end at now, got %v", r.To)
	}
}

func TestApplyUnknownPresetErrors(t *testing.T) {
	if _, err := newFixed().ApplyPreset("nope"); err == nil {
		t.Fatal("expected error for unknown preset")
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
	r, _ := s.ApplyPreset("15m")
	if len(s.Transfers(r)) == 0 {
		t.Fatal("expected at least one transfer in the last 15 minutes")
	}
}

func TestWiderPresetIncludesMoreOrEqual(t *testing.T) {
	s := newFixed()
	r15, _ := s.ApplyPreset("15m")
	r3mo, _ := s.ApplyPreset("3mo")
	if len(s.Transfers(r3mo)) < len(s.Transfers(r15)) {
		t.Fatal("3-month window must include at least as many transfers as 15-min")
	}
}

func TestSummaryBalancesAreConsistent(t *testing.T) {
	s := newFixed()
	r, _ := s.ApplyPreset("3mo")
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
	r, _ := s.ApplyPreset("3mo")
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

	if _, err := s.ApplyPreset("1h"); err != nil {
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

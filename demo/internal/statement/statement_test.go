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

func TestNewStartsEmpty(t *testing.T) {
	if got := newFixed().Count(); got != 0 {
		t.Fatalf("a new statement should hold no transfers, got %d", got)
	}
}

func TestSeedAddsTransfersInWindow(t *testing.T) {
	s := newFixed()
	if added := s.Seed(20, 15*time.Minute); added != 20 {
		t.Fatalf("Seed returned %d, want 20", added)
	}
	if s.Count() != 20 {
		t.Fatalf("count = %d, want 20 (table starts empty)", s.Count())
	}
	r, _ := s.ApplyRelative(15, "minutes")
	if got := len(s.Transfers(r)); got != 20 {
		t.Fatalf("expected 20 transfers in the last 15 min after seeding, got %d", got)
	}
}

func TestAllRangeListsEverything(t *testing.T) {
	s := newFixed()
	s.Seed(40, 90*24*time.Hour) // scattered over 90 days
	r, err := s.ApplyRelative(1, "all")
	if err != nil {
		t.Fatal(err)
	}
	if r.Label != "All transfers" {
		t.Fatalf("all range label = %q", r.Label)
	}
	if len(s.Transfers(r)) != s.Count() {
		t.Fatalf("'all' should list every transfer: got %d of %d", len(s.Transfers(r)), s.Count())
	}
}

func TestSeedPublishes(t *testing.T) {
	s := newFixed()
	ch := s.Events().Subscribe()
	defer s.Events().Unsubscribe(ch)
	s.Seed(5, time.Hour)
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("Seed should publish a range event")
	}
}

func TestWiderPresetIncludesMoreOrEqual(t *testing.T) {
	s := newFixed()
	s.Seed(200, 12*7*24*time.Hour)
	r15, _ := s.ApplyRelative(15, "minutes")
	r3mo, _ := s.ApplyRelative(12, "weeks")
	if len(s.Transfers(r3mo)) < len(s.Transfers(r15)) {
		t.Fatal("3-month window must include at least as many transfers as 15-min")
	}
}

func TestSummaryBucketsAndAvailable(t *testing.T) {
	s := newFixed()
	s.Seed(200, 12*7*24*time.Hour)
	r, _ := s.ApplyRelative(12, "weeks")
	sum := s.Summary(r)

	// Count includes RETURNED rows; buckets do not.
	if sum.Count != len(s.Transfers(r)) {
		t.Fatalf("summary count %d != transfers %d", sum.Count, len(s.Transfers(r)))
	}
	for _, v := range []int64{sum.CreditsPosted, sum.DebitsPosted, sum.CreditsPending, sum.DebitsPending} {
		if v < 0 {
			t.Fatalf("bucket should be non-negative: %+v", sum)
		}
	}
	// Available follows the unimatrix formula, clamped at zero.
	want := sum.CreditsPosted - sum.DebitsPosted - sum.DebitsPending
	if want < 0 {
		want = 0
	}
	if sum.AvailableMinor() != want {
		t.Fatalf("available = %d, want %d", sum.AvailableMinor(), want)
	}
}

func TestReturnedExcludedFromBuckets(t *testing.T) {
	s := newFixed()
	s.Seed(200, 12*7*24*time.Hour)
	r, _ := s.ApplyRelative(12, "weeks")
	sum := s.Summary(r)
	bucketTotal := sum.CreditsPosted + sum.DebitsPosted + sum.CreditsPending + sum.DebitsPending
	var bucketed, returned int64
	for _, tr := range s.Transfers(r) {
		if tr.Status == StatusReturned {
			returned += tr.AmountMinor
		} else {
			bucketed += tr.AmountMinor
		}
	}
	if bucketTotal != bucketed {
		t.Fatalf("bucket total %d != non-returned total %d", bucketTotal, bucketed)
	}
	if returned == 0 {
		t.Skip("no returned transfers in window to exclude")
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

package store

import (
	"testing"
	"time"
)

func TestSetGet(t *testing.T) {
	s := New()
	s.Set("foo", "bar")
	v, ok := s.Get("foo")
	if !ok || v != "bar" {
		t.Fatalf("expected (bar, true), got (%q, %v)", v, ok)
	}
}

func TestGetMissing(t *testing.T) {
	s := New()
	_, ok := s.Get("nope")
	if ok {
		t.Fatal("expected missing key to return ok=false")
	}
}

func TestAll(t *testing.T) {
	s := New()
	s.Set("a", "1")
	s.Set("b", "2")
	all := s.All()
	if len(all) != 2 || all["a"] != "1" || all["b"] != "2" {
		t.Fatalf("unexpected All() result: %v", all)
	}
}

func TestAllReturnsSnapshot(t *testing.T) {
	s := New()
	s.Set("x", "1")
	snapshot := s.All()
	s.Set("x", "2")
	if snapshot["x"] != "1" {
		t.Fatal("All() must return a copy, not a live reference")
	}
}

func TestVersionIncrements(t *testing.T) {
	s := New()
	s.Set("k", "v1")
	v1 := s.Version("k")
	s.Set("k", "v2")
	v2 := s.Version("k")
	if v1 == 0 || v2 == 0 || v2 <= v1 {
		t.Fatalf("version should increment: v1=%d v2=%d", v1, v2)
	}
}

func TestVersionMissingKeyIsZero(t *testing.T) {
	s := New()
	if v := s.Version("missing"); v != 0 {
		t.Fatalf("expected version 0 for absent key, got %d", v)
	}
}

func TestSetIfSucceedsWithMatchingVersion(t *testing.T) {
	s := New()
	s.Set("k", "v1")
	ver := s.Version("k")

	newVer, ok := s.SetIf("k", "v2", "test", ver)
	if !ok {
		t.Fatal("SetIf should succeed when version matches")
	}
	if newVer <= ver {
		t.Fatalf("new version %d should be greater than %d", newVer, ver)
	}
	v, _ := s.Get("k")
	if v != "v2" {
		t.Fatalf("expected v2, got %q", v)
	}
}

func TestSetIfFailsOnVersionConflict(t *testing.T) {
	s := New()
	s.Set("k", "v1")
	ver := s.Version("k")
	s.Set("k", "v2") // bumps version

	cur, ok := s.SetIf("k", "v3", "test", ver) // stale version
	if ok {
		t.Fatal("SetIf should fail when version does not match")
	}
	if cur <= ver {
		t.Fatalf("returned version %d should be > stale version %d", cur, ver)
	}
	v, _ := s.Get("k")
	if v != "v2" {
		t.Fatalf("value should be unchanged (v2), got %q", v)
	}
}

func TestSetIfWithZeroVersionAlwaysWrites(t *testing.T) {
	s := New()
	s.Set("k", "v1")
	_, ok := s.SetIf("k", "v2", "test", 0)
	if !ok {
		t.Fatal("SetIf with version=0 should always succeed")
	}
}

func TestSetIfRecordsSource(t *testing.T) {
	s := New()
	s.SetIf("k", "v", "datastar", 0)
	entries := s.Entries()
	if len(entries) != 1 || entries[0].Source != "datastar" {
		t.Fatalf("expected source datastar, got %+v", entries)
	}
}

func TestEntriesSortedByKey(t *testing.T) {
	s := New()
	s.Set("b", "2")
	s.Set("a", "1")
	entries := s.Entries()
	if len(entries) != 2 || entries[0].Key != "a" || entries[1].Key != "b" {
		t.Fatalf("expected sorted entries, got %+v", entries)
	}
}

func TestDeleteRemovesKeyAndNotifies(t *testing.T) {
	s := New()
	s.Set("k", "v")
	ch := s.Subscribe()
	defer s.Unsubscribe(ch)

	if !s.Delete("k", "htmx") {
		t.Fatal("Delete should return true for existing key")
	}
	if _, ok := s.Get("k"); ok {
		t.Fatal("key should be gone after Delete")
	}
	select {
	case e := <-ch:
		if !e.Deleted || e.Key != "k" || e.Source != "htmx" {
			t.Fatalf("unexpected delete event %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delete event")
	}
}

func TestDeleteMissingKeyReturnsFalse(t *testing.T) {
	s := New()
	if s.Delete("nope", "htmx") {
		t.Fatal("Delete of missing key should return false")
	}
}

func TestAllStatesIncludesVersion(t *testing.T) {
	s := New()
	s.Set("a", "1")
	states := s.AllStates()
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	if states[0].Key != "a" || states[0].Value != "1" || states[0].Version == 0 {
		t.Fatalf("unexpected state: %+v", states[0])
	}
}

func TestSeqIsMonotonic(t *testing.T) {
	s := New()
	ch := s.Subscribe()
	defer s.Unsubscribe(ch)

	s.Set("a", "1")
	s.Set("b", "2")

	e1 := <-ch
	e2 := <-ch
	if e1.Seq >= e2.Seq {
		t.Fatalf("seq must be monotonic: %d >= %d", e1.Seq, e2.Seq)
	}
}

func TestSubscribeReceivesEvent(t *testing.T) {
	s := New()
	ch := s.Subscribe()
	defer s.Unsubscribe(ch)

	s.Set("key", "val")
	select {
	case e := <-ch:
		if e.Key != "key" || e.Value != "val" {
			t.Fatalf("unexpected event %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestUnsubscribeDoesNotBlock(t *testing.T) {
	s := New()
	ch := s.Subscribe()
	s.Unsubscribe(ch)
	s.Set("key", "val") // must not block or panic
}

func TestMultipleSubscribers(t *testing.T) {
	s := New()
	ch1 := s.Subscribe()
	ch2 := s.Subscribe()
	defer s.Unsubscribe(ch1)
	defer s.Unsubscribe(ch2)

	s.Set("k", "v")
	for _, ch := range []chan Event{ch1, ch2} {
		select {
		case e := <-ch:
			if e.Key != "k" {
				t.Fatalf("unexpected key %q", e.Key)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	}
}

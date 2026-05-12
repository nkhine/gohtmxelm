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

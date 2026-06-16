package gohtmxelm

import (
	"sync"
	"testing"
	"testing/synctest"
)

// These tests drive the REAL Broadcaster's goroutines and channels under
// testing/synctest, which gives deterministic control of concurrency without
// the single-threaded modelling the simnet harness uses. They are the half of
// the strategy simnet structurally cannot do: exercise the shipped code, with
// -race catching data races the model can't see.

// TestBroadcaster_FanOut: every subscriber present at publish time receives
// every value, in order, when buffers are ample.
func TestBroadcaster_FanOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBroadcaster[int](16)
		const subs, vals = 4, 10

		var wg sync.WaitGroup
		got := make([][]int, subs)
		for i := range subs {
			ch := b.Subscribe()
			wg.Add(1)
			go func() {
				defer wg.Done()
				for v := range ch {
					got[i] = append(got[i], v)
				}
			}()
		}

		// Let subscribers reach their receive before publishing.
		synctest.Wait()
		for v := 1; v <= vals; v++ {
			b.Publish(v)
		}
		synctest.Wait() // all values drained into the goroutines

		// Unsubscribe each channel to close it and end its ranging goroutine.
		b.mu.RLock()
		chans := make([]chan int, 0, len(b.subs))
		for ch := range b.subs {
			chans = append(chans, ch)
		}
		b.mu.RUnlock()
		for _, ch := range chans {
			b.Unsubscribe(ch)
		}
		wg.Wait()

		for i := range subs {
			if len(got[i]) != vals {
				t.Errorf("subscriber %d got %d values, want %d", i, len(got[i]), vals)
				continue
			}
			for j, v := range got[i] {
				if v != j+1 {
					t.Errorf("subscriber %d out of order at %d: got %d", i, j, v)
					break
				}
			}
		}
	})
}

// TestBroadcaster_PublishNeverBlocks: with a full buffer and no reader,
// Publish drops rather than blocking — the lossy contract simnet models. If
// Publish blocked, synctest would detect the deadlock and fail.
func TestBroadcaster_PublishNeverBlocks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBroadcaster[int](2)
		ch := b.Subscribe()

		// Publish well past the buffer with nobody draining ch.
		for v := 1; v <= 100; v++ {
			b.Publish(v)
		}
		synctest.Wait()

		if got := len(ch); got != 2 {
			t.Fatalf("buffer occupancy = %d, want 2 (excess must be dropped)", got)
		}
		// The two buffered values are the FIRST two published; later ones were
		// dropped at the non-blocking send.
		if v := <-ch; v != 1 {
			t.Errorf("first buffered = %d, want 1", v)
		}
		if v := <-ch; v != 2 {
			t.Errorf("second buffered = %d, want 2", v)
		}
		b.Unsubscribe(ch)
	})
}

// TestBroadcaster_UnsubscribeCloses: Unsubscribe removes and closes the channel
// so a ranging consumer terminates cleanly.
func TestBroadcaster_UnsubscribeCloses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBroadcaster[string](4)
		ch := b.Subscribe()

		done := make(chan bool)
		go func() {
			for range ch {
			}
			done <- true
		}()

		synctest.Wait()
		b.Unsubscribe(ch)

		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("consumer did not exit after Unsubscribe closed the channel")
		}

		// Double-unsubscribe must be safe.
		b.Unsubscribe(ch)
	})
}

// TestBroadcaster_ConcurrentPubSub hammers Subscribe/Unsubscribe/Publish from
// many goroutines at once. Its real value is under `go test -race`: it asserts
// the mutex discipline in broadcaster.go is sound. synctest makes the wind-down
// deterministic.
func TestBroadcaster_ConcurrentPubSub(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := NewBroadcaster[int](8)

		var wg sync.WaitGroup
		// Publishers.
		for p := range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range 50 {
					b.Publish(p*100 + i)
				}
			}()
		}
		// Subscribers that join, drain a little, and leave.
		for range 6 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ch := b.Subscribe()
				defer b.Unsubscribe(ch)
				for n := 0; n < 20; n++ {
					select {
					case <-ch:
					default:
						return
					}
				}
			}()
		}

		wg.Wait()
		synctest.Wait()
		// No assertion on values (delivery is lossy under contention); the test
		// passes if -race is clean and synctest sees no deadlock.
	})
}

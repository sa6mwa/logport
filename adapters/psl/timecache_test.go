package psl

import (
	"testing"
	"time"
)

func TestTimeCacheCachesWithinTick(t *testing.T) {
	layout := time.RFC3339Nano
	start := time.Date(2025, time.October, 12, 12, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	tickCh := make(chan time.Time, 1)

	cache := &timeCache{
		layout:    layout,
		now:       func() time.Time { return start },
		newTicker: func(time.Duration) tickerControl { return tickerControl{C: tickCh} },
	}

	first := cache.Current()
	wantFirst := start.Format(layout)
	if first != wantFirst {
		t.Fatalf("initial cache value mismatch: got %q want %q", first, wantFirst)
	}

	next := start.Add(500 * time.Millisecond)
	cache.now = func() time.Time { return next }
	second := cache.Current()
	if second != first {
		t.Fatalf("cache should return cached value before tick: got %q want %q", second, first)
	}

	advance := start.Add(time.Second)
	tickCh <- advance
	close(tickCh)

	wantAdvance := advance.Format(layout)
	deadline := time.After(200 * time.Millisecond)
	for {
		current := cache.Current()
		if current == wantAdvance {
			break
		}
		select {
		case <-time.After(5 * time.Millisecond):
		case <-deadline:
			t.Fatalf("cache did not update after tick; last value %q, want %q", current, wantAdvance)
		}
	}
}

func TestTimeCacheHonoursUTC(t *testing.T) {
	layout := time.RFC3339Nano
	start := time.Date(2025, time.July, 4, 15, 30, 0, 0, time.FixedZone("PDT", -7*3600))
	tickCh := make(chan time.Time, 1)

	cache := &timeCache{
		layout: layout,
		utc:    true,
		now:    func() time.Time { return start },
		newTicker: func(time.Duration) tickerControl {
			return tickerControl{C: tickCh}
		},
	}

	initial := cache.Current()
	if initial != start.UTC().Format(layout) {
		t.Fatalf("expected UTC formatting on initial value: got %q want %q", initial, start.UTC().Format(layout))
	}

	next := start.Add(2 * time.Second)
	tickCh <- next
	close(tickCh)

	want := next.UTC().Format(layout)
	deadline := time.After(200 * time.Millisecond)
	for {
		current := cache.Current()
		if current == want {
			break
		}
		select {
		case <-time.After(5 * time.Millisecond):
		case <-deadline:
			t.Fatalf("expected UTC formatting after tick; last %q want %q", current, want)
		}
	}
}

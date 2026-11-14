package ratelimit

import (
	"testing"
	"time"
)

// clock is a time source the test moves by hand, so no test sleeps.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newLimiter(limit int, per time.Duration) (*Limiter, *clock) {
	c := &clock{t: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
	l := New(limit, per)
	l.now = c.now
	return l, c
}

func TestTheWholeAllowanceCanBeSpentAtOnce(t *testing.T) {
	l, _ := newLimiter(3, time.Minute)
	for i := range 3 {
		if ok, _ := l.Allow("alice"); !ok {
			t.Fatalf("request %d of 3 was refused", i+1)
		}
	}
	ok, wait := l.Allow("alice")
	if ok {
		t.Fatal("a fourth request inside the minute was allowed")
	}
	// 3 a minute is one every 20 seconds, and the bucket is exactly empty.
	if wait != 20*time.Second {
		t.Errorf("retry after = %s, want 20s", wait)
	}
}

func TestTokensComeBackEvenly(t *testing.T) {
	l, c := newLimiter(3, time.Minute)
	for range 3 {
		l.Allow("alice")
	}

	c.advance(19 * time.Second)
	if ok, wait := l.Allow("alice"); ok || wait != time.Second {
		t.Fatalf("after 19s: allowed=%v wait=%s, want refused with 1s left", ok, wait)
	}
	c.advance(time.Second)
	if ok, _ := l.Allow("alice"); !ok {
		t.Fatal("after 20s one token should be back")
	}
	if ok, _ := l.Allow("alice"); ok {
		t.Fatal("only one token came back, but two requests were allowed")
	}

	// However long the wait, the bucket holds no more than its limit.
	c.advance(time.Hour)
	for i := range 3 {
		if ok, _ := l.Allow("alice"); !ok {
			t.Fatalf("after an hour, request %d of 3 was refused", i+1)
		}
	}
	if ok, _ := l.Allow("alice"); ok {
		t.Fatal("an idle hour bought more than the limit")
	}
}

func TestKeysDoNotShareAnAllowance(t *testing.T) {
	l, _ := newLimiter(1, time.Minute)
	if ok, _ := l.Allow("alice"); !ok {
		t.Fatal("alice's first request was refused")
	}
	if ok, _ := l.Allow("alice"); ok {
		t.Fatal("alice's second request was allowed")
	}
	if ok, _ := l.Allow("bob"); !ok {
		t.Fatal("bob was refused because alice spent hers")
	}
}

func TestRetryAfterIsNeverLessThanASecond(t *testing.T) {
	// 600 a minute is ten a second: the true wait is 100 ms.
	l, _ := newLimiter(600, time.Minute)
	for range 600 {
		l.Allow("alice")
	}
	if ok, wait := l.Allow("alice"); ok || wait != time.Second {
		t.Fatalf("allowed=%v wait=%s, want refused with 1s", ok, wait)
	}
}

func TestIdleKeysAreForgotten(t *testing.T) {
	l, c := newLimiter(2, time.Minute)
	l.Allow("gone")
	c.advance(30 * time.Second)
	l.Allow("recent")

	// A minute after "gone" was last seen its bucket is full again, so there
	// is nothing to remember. "recent" is only half a minute old.
	c.advance(30 * time.Second)
	l.Allow("other")

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, kept := l.buckets["gone"]; kept {
		t.Error("a key idle for the whole window is still held")
	}
	if _, kept := l.buckets["recent"]; !kept {
		t.Error("a key seen half a window ago was forgotten, and with it what it had spent")
	}
}

func TestALimitBelowOneStillAllowsOne(t *testing.T) {
	l, _ := newLimiter(0, time.Minute)
	if ok, _ := l.Allow("alice"); !ok {
		t.Fatal("a limiter built with 0 refuses everything")
	}
}

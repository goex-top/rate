package rate

import (
	"container/list"
	"sync"
	"time"
)

// A RateLimiter limits the rate at which an action can be performed.  It
// applies neither smoothing (like one could achieve in a token bucket system)
// nor does it offer any conception of warmup, wherein the rate of actions
// granted are steadily increased until a steady throughput equilibrium is
// reached.
type RateLimiter struct {
	limit     int
	remaining int
	resetAt   time.Time
	interval  time.Duration
	wait      time.Duration
	mtx       sync.RWMutex
	times     list.List
}

// New creates a new rate limiter for the limit and interval.
func New(limit int, interval time.Duration) *RateLimiter {
	lim := &RateLimiter{
		limit:     limit,
		remaining: limit,
		interval:  interval,
	}
	lim.times.Init()
	return lim
}

// Wait blocks if the rate limit has been reached.  Wait offers no guarantees
// of fairness for multiple actors if the allowed rate has been temporarily
// exhausted.
func (r *RateLimiter) Wait() {
	for {
		ok, wait, _ := r.Try()
		if ok {
			break
		}
		time.Sleep(wait)
	}
}

func (r *RateLimiter) cleanTimes(now *time.Time) {
	for e := r.times.Front(); e != nil; e = e.Next() {
		if diff := now.Sub(e.Value.(time.Time)); diff >= r.interval {
			r.times.Remove(e)
		}
  }
}

func (r *RateLimiter) Reverse() {
	defer r.mtx.Unlock()
	r.mtx.Lock()
	back := r.times.Back()
	if back != nil {
		r.times.Remove(back)
	}
}

// Try returns true if under the rate limit, or false if over and the
// remaining time before the rate limit expires.
func (r *RateLimiter) Try() (ok bool, wait time.Duration, remain int) {
	defer r.mtx.Unlock()
	r.mtx.Lock()
	r.wait = 0
	now := time.Now()
	r.cleanTimes(&now)

	if l := r.times.Len(); l < r.limit {
		r.remaining = r.limit - l - 1
		r.times.PushBack(now)
		return true, 0, r.remaining
	}

	r.remaining = r.limit - r.times.Len()

	frnt := r.times.Front()
	if diff := now.Sub(frnt.Value.(time.Time)); diff <= r.interval {
		r.wait = r.interval - diff
		r.resetAt = now.Add(r.wait)
		return false, r.wait, r.remaining
	}

	frnt.Value = now
	r.times.MoveToBack(frnt)
	return true, 0, r.remaining
}

func (r *RateLimiter) SetRemaining(remaining int) {
	defer r.mtx.Unlock()
	r.mtx.Lock()
	now := time.Now()
	r.cleanTimes(&now)
	r.remaining = r.limit - r.times.Len()
	diff := r.remaining - remaining
	if diff > 0 {
		for i := 0; i < diff; i++ {
			r.times.PushBack(now)
		}
	} else if diff < 0 {
		e := r.times.Front()
		for i := 0; i < -diff; i++ {
			if e != nil {
				r.times.Remove(e)
				e = e.Next()
			}
		}
	}
	r.remaining = remaining
}

func (r *RateLimiter) Remaining() int {
	defer r.mtx.RUnlock()
	r.mtx.RLock()
	return r.remaining
}

func (r *RateLimiter) UpdateRemaining() int {
	defer r.mtx.Unlock()
	r.mtx.Lock()
	now := time.Now()
	for e := r.times.Front(); e != nil; e = e.Next() {
		if diff := now.Sub(e.Value.(time.Time)); diff > r.interval {
			r.times.Remove(e)
		}
	}
	r.remaining = r.limit - r.times.Len()

	return r.remaining
}

func (r *RateLimiter) Limit() int {
	defer r.mtx.RUnlock()
	r.mtx.RLock()
	return r.limit
}

func (r *RateLimiter) ResetAt() time.Time {
	defer r.mtx.RUnlock()
	r.mtx.RLock()
	return r.resetAt
}

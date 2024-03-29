// This Backoff interface implements an exponential backoff algorithm.
package utils

import (
	"math"
	"math/rand"
	"sync/atomic"
	"time"
)

// Backoff is a time.Duration counter, has a value between Min and Max.
// After every call to the Duration method the current timing is
// multiplied by |Factor|, but it never exceeds Max.
//
// Backoff is not generally concurrent-safe, but the ForAttempt method can
// be used concurrently.
type Backoff struct {
	attempt uint64
	// Factor is the multiplying factor for each increment step
	Factor float64
	// Jitter eases contention by randomizing backoff steps
	Jitter bool
	// Min and Max are the minimum and maximum values of the counter
	Min, Max time.Duration
}

// Return the duration for the current attempt before incrementing
// the attempt counter.
func (b *Backoff) Duration() time.Duration {
	d := b.ForAttempt(float64(atomic.AddUint64(&b.attempt, 1) - 1))
	return d
}

const maxInt64 = float64(math.MaxInt64 - 512)

// Return the duration for a specific attempt. This function is useful if
// you have a large number of independent Backoffs, but don't want use
// unnecessary memory storing the Backoff parameters per Backoff. The first
// attempt should be 0.
//
// ForAttempt is concurrent-safe.
func (b *Backoff) ForAttempt(attempt float64) time.Duration {
	min := b.Min
	if min <= 0 {
		min = 100 * time.Millisecond
	}
	max := b.Max
	if max <= 0 {
		max = 10 * time.Second
	}
	if min >= max {
		return max
	}
	factor := b.Factor
	if factor <= 0 {
		factor = 2
	}

	minf := float64(min)
	durf := minf * math.Pow(factor, attempt)
	if b.Jitter {
		durf = rand.Float64()*(durf-minf) + minf
	}

	// Check that float64 wont overflow int64
	if durf > maxInt64 {
		return max
	}

	dur := time.Duration(durf)
	//keep within bounds
	if dur < min {
		return min
	}
	if dur > max {
		return max
	}
	return dur
}

// Restart the |attempt| counter at zero.
func (b *Backoff) Reset() {
	atomic.StoreUint64(&b.attempt, 0)
}

// Return the current attempt counter value.
func (b *Backoff) Attempt() float64 {
	return float64(atomic.LoadUint64(&b.attempt))
}

// Copy and return a backoff obj
func (b *Backoff) Copy() *Backoff {
	return &Backoff{
		Factor: b.Factor,
		Jitter: b.Jitter,
		Min:    b.Min,
		Max:    b.Max,
	}
}

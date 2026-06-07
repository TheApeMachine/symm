package runstats

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
)

/*
Watchdog turns silence into noise. Every serious data-layer failure in this
system's history was silent — the quote cache severed from the bus while signals
kept flowing, the audit stream stopping mid-session, a latency profile of zeros —
because nothing watched for inputs that SHOULD be flowing but aren't. Components
register expectations; the watchdog evaluates them on a cadence and escalates
violations (log + optional trip callback, e.g. the desk circuit breaker).
*/
type Watchdog struct {
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	expectations []*expectation
	interval     time.Duration
	onTrip       func(name string, detail string)
}

type expectation struct {
	name     string
	check    func() (ok bool, detail string)
	grace    time.Duration
	armedAt  time.Time
	lastOK   time.Time
	tripped  bool
	critical bool
}

/*
NewWatchdog builds a watchdog that evaluates expectations every interval. onTrip
runs once per expectation transition into violation; pass the desk's TripHalt for
critical expectations to stop order flow when the data layer goes dark.
*/
func NewWatchdog(
	ctx context.Context,
	interval time.Duration,
	onTrip func(name string, detail string),
) *Watchdog {
	ctx, cancel := context.WithCancel(ctx)

	if interval <= 0 {
		interval = 10 * time.Second
	}

	return &Watchdog{
		ctx:      ctx,
		cancel:   cancel,
		interval: interval,
		onTrip:   onTrip,
	}
}

/*
Expect registers a named expectation. check must be cheap and non-blocking; it
returns ok=false with a human detail when violated. grace is how long a violation
must persist before escalation (one slow tick is not an incident). critical marks
expectations whose violation should trip the halt callback, not just log.
*/
func (watchdog *Watchdog) Expect(
	name string,
	grace time.Duration,
	critical bool,
	check func() (bool, string),
) {
	if watchdog == nil || check == nil || name == "" {
		return
	}

	watchdog.mu.Lock()
	defer watchdog.mu.Unlock()

	now := time.Now()
	watchdog.expectations = append(watchdog.expectations, &expectation{
		name:     name,
		check:    check,
		grace:    grace,
		armedAt:  now,
		lastOK:   now,
		critical: critical,
	})
}

/*
Tick runs the evaluation loop until the context ends. It satisfies the engine's
System contract so the watchdog slots in beside every other subsystem.
*/
func (watchdog *Watchdog) Tick() error {
	ticker := time.NewTicker(watchdog.interval)
	defer ticker.Stop()

	for {
		select {
		case <-watchdog.ctx.Done():
			return watchdog.ctx.Err()
		case <-ticker.C:
			watchdog.evaluate(time.Now())
		}
	}
}

func (watchdog *Watchdog) evaluate(now time.Time) {
	watchdog.mu.Lock()
	expectations := make([]*expectation, len(watchdog.expectations))
	copy(expectations, watchdog.expectations)
	watchdog.mu.Unlock()

	for _, expect := range expectations {
		ok, detail := expect.check()

		if ok {
			expect.lastOK = now

			if expect.tripped {
				expect.tripped = false
				errnie.Info(
					fmt.Sprintf("watchdog: %s recovered", expect.name),
					"runstats/watchdog",
				)
			}

			continue
		}

		if now.Sub(expect.lastOK) < expect.grace {
			continue
		}

		if expect.tripped {
			continue
		}

		expect.tripped = true
		errnie.Error(fmt.Errorf(
			"watchdog: %s violated for %s: %s",
			expect.name,
			now.Sub(expect.lastOK).Round(time.Second),
			detail,
		), "runstats/watchdog")

		if expect.critical && watchdog.onTrip != nil {
			watchdog.onTrip(expect.name, detail)
		}
	}
}

func (watchdog *Watchdog) Close() error {
	watchdog.cancel()

	return nil
}

/*
RateExpectation adapts a monotonically increasing counter into a "this must keep
moving while that other counter moves" check — the exact shape of the quote-cache
starvation incident: raw frames flowed everywhere except into the cache.
*/
func RateExpectation(
	observed func() uint64,
	reference func() uint64,
) func() (bool, string) {
	var lastObserved, lastReference uint64
	primed := false

	return func() (bool, string) {
		currentObserved := observed()
		currentReference := reference()

		defer func() {
			lastObserved = currentObserved
			lastReference = currentReference
			primed = true
		}()

		if !primed {
			return true, ""
		}

		referenceMoved := currentReference > lastReference
		observedMoved := currentObserved > lastObserved

		if !referenceMoved || observedMoved {
			return true, ""
		}

		return false, fmt.Sprintf(
			"reference advanced (%d→%d) while observed stalled at %d",
			lastReference, currentReference, currentObserved,
		)
	}
}

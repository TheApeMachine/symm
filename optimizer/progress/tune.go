package progress

import "time"

const tuneProgressMinSpacing = 3 * time.Second

/*
TuneProgressInterval chooses a step size that yields roughly twenty updates
across a long phase without spamming stderr on small workloads.
*/
func TuneProgressInterval(total int) int {
	if total <= 1 {
		return 1
	}

	interval := total / 20

	if interval < 1 {
		return 1
	}

	if interval > 512 {
		return 512
	}

	return interval
}

/*
TuneProgressReporter emits periodic tune progress for counted long-running phases.
It logs the first and last steps, every interval boundary, and at least once
every tuneProgressMinSpacing when work is still running.
*/
type TuneProgressReporter struct {
	total      int
	interval   int
	started    time.Time
	lastLogged time.Time
}

func NewTuneProgressReporter(total int) *TuneProgressReporter {
	now := time.Now()

	return &TuneProgressReporter{
		total:      total,
		interval:   TuneProgressInterval(total),
		started:    now,
		lastLogged: now,
	}
}

func (reporter *TuneProgressReporter) ShouldLog(completed int) bool {
	if completed <= 0 {
		return false
	}

	if completed == 1 || completed == reporter.total {
		return true
	}

	if reporter.interval > 0 && completed%reporter.interval == 0 {
		return true
	}

	if time.Since(reporter.lastLogged) >= tuneProgressMinSpacing {
		return true
	}

	return false
}

func (reporter *TuneProgressReporter) MarkLogged() {
	reporter.lastLogged = time.Now()
}

func (reporter *TuneProgressReporter) Elapsed() time.Duration {
	return time.Since(reporter.started).Round(time.Millisecond)
}

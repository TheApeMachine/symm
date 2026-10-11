package audit

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
)

type progressTracker struct {
	label     string
	total     int64
	startTime time.Time
	lastLog   time.Time
	interval  time.Duration
	completed bool
	mutex     sync.Mutex
}

func newProgressTracker(label string, total int64, interval time.Duration) *progressTracker {
	now := time.Now()
	return &progressTracker{
		label:     label,
		total:     total,
		startTime: now,
		lastLog:   now,
		interval:  interval,
	}
}

func (tracker *progressTracker) Step(completed int64, extra ...string) {
	if tracker == nil || tracker.total <= 0 {
		return
	}

	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()

	if tracker.completed {
		return
	}

	now := time.Now()
	if completed < tracker.total && now.Sub(tracker.lastLog) < tracker.interval {
		return
	}

	if completed >= tracker.total {
		tracker.completed = true
		completed = tracker.total
	}

	tracker.lastLog = now
	elapsed := now.Sub(tracker.startTime)
	percent := float64(completed) / float64(tracker.total) * 100.0

	var etaStr string
	if completed > 0 && completed < tracker.total {
		rate := float64(completed) / elapsed.Seconds()
		if rate > 0 {
			remainingSec := float64(tracker.total-completed) / rate
			etaStr = fmt.Sprintf(" | ETA: %s", (time.Duration(remainingSec) * time.Second).Round(time.Second))
		}
	}

	extraStr := ""
	if len(extra) > 0 && extra[0] != "" {
		extraStr = " | " + extra[0]
	}

	errnie.Info(fmt.Sprintf(
		"[audit] [%s] %d / %d (%.1f%%) | elapsed: %s%s%s",
		tracker.label, completed, tracker.total, percent, elapsed.Round(time.Second), etaStr, extraStr,
	))
}

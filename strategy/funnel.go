package strategy

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/runtime"
)

const (
	funnelLogEvery   = 500
	funnelLogMinGap  = 10 * time.Second
	funnelTopReasons = 12
)

/*
funnel tallies Step seals so an 80k-tick dry spell is diagnosable without
per-symbol spam. emit() dumps top Gate/Reason buckets plus candidate/enter
counts on a tick+time cadence.
*/
type funnel struct {
	mu        sync.Mutex
	ticks     int64
	lastEmit  time.Time
	skips     map[string]int
	enters    int
	enterFail map[string]int
	cands     int
}

func newFunnel() *funnel {
	return &funnel{
		skips:     make(map[string]int),
		enterFail: make(map[string]int),
	}
}

func (funnel *funnel) observe(
	envelope *runtime.Envelope,
	candidates int,
	cash float64,
	open int,
) {
	if funnel == nil {
		return
	}

	funnel.mu.Lock()
	defer funnel.mu.Unlock()

	funnel.ticks++
	funnel.cands += candidates

	if envelope != nil {
		for _, decision := range envelope.Decisions {
			if decision == nil {
				continue
			}

			switch decision.Action {
			case "enter":
				funnel.enters++
			case "skip":
				key := funnelKey(decision.Gate, decision.Reason)
				funnel.skips[key]++
			}
		}
	}

	now := time.Now()
	if funnel.ticks%funnelLogEvery != 0 && now.Sub(funnel.lastEmit) < funnelLogMinGap {
		return
	}

	funnel.emitLocked(cash, open)
	funnel.lastEmit = now
}

func (funnel *funnel) failedEnter(err error) {
	if funnel == nil || err == nil {
		return
	}

	funnel.mu.Lock()
	defer funnel.mu.Unlock()

	funnel.enterFail[shortErr(err)]++
}

func (funnel *funnel) emitLocked(cash float64, open int) {
	type pair struct {
		Key string
		N   int
	}

	skips := make([]pair, 0, len(funnel.skips))
	for key, n := range funnel.skips {
		skips = append(skips, pair{Key: key, N: n})
	}
	sort.Slice(skips, func(i, j int) bool {
		if skips[i].N == skips[j].N {
			return skips[i].Key < skips[j].Key
		}
		return skips[i].N > skips[j].N
	})
	if len(skips) > funnelTopReasons {
		skips = skips[:funnelTopReasons]
	}

	fails := make([]pair, 0, len(funnel.enterFail))
	for key, n := range funnel.enterFail {
		fails = append(fails, pair{Key: key, N: n})
	}
	sort.Slice(fails, func(i, j int) bool {
		if fails[i].N == fails[j].N {
			return fails[i].Key < fails[j].Key
		}
		return fails[i].N > fails[j].N
	})
	if len(fails) > funnelTopReasons {
		fails = fails[:funnelTopReasons]
	}

	skipParts := make([]string, 0, len(skips))
	for _, row := range skips {
		skipParts = append(skipParts, fmt.Sprintf("%s=%d", row.Key, row.N))
	}
	failParts := make([]string, 0, len(fails))
	for _, row := range fails {
		failParts = append(failParts, fmt.Sprintf("%s=%d", row.Key, row.N))
	}

	clear(funnel.skips)
	clear(funnel.enterFail)
	funnel.cands = 0
	funnel.enters = 0
}

func funnelKey(gate, reason string) string {
	if gate == "" {
		gate = "?"
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "?"
	}
	// Board embeds score floats — collapse to the stem for aggregation.
	if i := strings.Index(reason, " (score="); i >= 0 {
		reason = reason[:i]
	}
	if len(reason) > 96 {
		reason = reason[:96]
	}
	return gate + ": " + reason
}

func shortErr(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, "why"); i >= 0 {
		msg = strings.TrimSpace(msg[i:])
	}
	if len(msg) > 120 {
		msg = msg[:120]
	}
	return msg
}

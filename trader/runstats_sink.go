package trader

import "github.com/theapemachine/symm/runstats"

/*
statsSink adapts the trader's runStats counters to the runstats.Sink
interface so packages outside trader can record events without taking a
dependency on trader. The sink is installed in NewCrypto so the
counters are available the moment the trader is wired up.
*/
type statsSink struct{}

func (statsSink) UIFramesSent(n int64)     { stats.uiFramesSent.Add(n) }
func (statsSink) UIFramesDropped(n int64)  { stats.uiFramesDropped.Add(n) }
func (statsSink) UIFramesFiltered(n int64) { stats.uiFramesFiltered.Add(n) }
func (statsSink) LeadlagThrottle()         { stats.leadlagThrottleHits.Add(1) }
func (statsSink) LeadlagRecompute()        { stats.leadlagRecomputes.Add(1) }
func (statsSink) WSConnect()               { stats.wsConnects.Add(1) }
func (statsSink) WSReconnect()             { stats.wsReconnects.Add(1) }
func (statsSink) TokenRefresh(success bool) {
	if success {
		stats.tokenRefreshes.Add(1)

		return
	}

	stats.tokenRefreshFail.Add(1)
}

func init() {
	runstats.Install(statsSink{})
}

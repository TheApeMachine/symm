package public

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/numeric"
)

const rttSampleWindow = 256

/*
NetworkLatency tracks measured Kraken websocket RTT and exposes one-way latency
for paper fill and order-activation timing.
*/
type NetworkLatency struct {
	mu         sync.RWMutex
	lastRTT    time.Duration
	p95RTT     time.Duration
	rttSamples []time.Duration
	samples    uint64
	updated    time.Time
}

var sharedNetworkLatency = NewNetworkLatency()

func NewNetworkLatency() *NetworkLatency {
	return &NetworkLatency{}
}

func SharedNetworkLatency() *NetworkLatency {
	return sharedNetworkLatency
}

func (latency *NetworkLatency) LoadProfile(profile LatencyProfile) {
	if profile.RTT() <= 0 && profile.P95RTT() <= 0 {
		return
	}

	latency.mu.Lock()
	defer latency.mu.Unlock()

	latency.lastRTT = profile.RTT()
	latency.p95RTT = profile.P95RTT()
	latency.samples = profile.Samples

	if latency.p95RTT > 0 {
		latency.rttSamples = []time.Duration{latency.p95RTT}
	}

	if !profile.UpdatedAt.IsZero() {
		latency.updated = profile.UpdatedAt
	}
}

func (latency *NetworkLatency) Reset() {
	latency.mu.Lock()
	defer latency.mu.Unlock()

	latency.lastRTT = 0
	latency.p95RTT = 0
	latency.rttSamples = nil
	latency.samples = 0
	latency.updated = time.Time{}
}

func BootstrapNetworkLatency() {
	if profile, ok := LoadLatencyProfile(); ok {
		SharedNetworkLatency().LoadProfile(profile)
	}
}

func (latency *NetworkLatency) RecordRTT(roundTrip time.Duration) {
	if roundTrip <= 0 {
		return
	}

	latency.mu.Lock()
	defer latency.mu.Unlock()

	if latency.samples == 0 {
		latency.lastRTT = roundTrip
	} else {
		alpha := 2.0 / (float64(latency.samples) + 1)
		latency.lastRTT = time.Duration(
			float64(latency.lastRTT)*(1-alpha) + float64(roundTrip)*alpha,
		)
	}

	latency.samples++
	latency.updated = time.Now().UTC()
	latency.appendRTTSample(roundTrip)
	latency.persistProfileLocked()
}

func (latency *NetworkLatency) appendRTTSample(roundTrip time.Duration) {
	latency.rttSamples = append(latency.rttSamples, roundTrip)

	if len(latency.rttSamples) > rttSampleWindow {
		offset := len(latency.rttSamples) - rttSampleWindow
		latency.rttSamples = latency.rttSamples[offset:]
	}

	latency.p95RTT = rttP95(latency.rttSamples)
}

func rttP95(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}

	values := make([]float64, len(samples))

	for index, sample := range samples {
		values[index] = float64(sample)
	}

	sorted := numeric.CopySorted(values)

	return time.Duration(numeric.PercentileSorted(sorted, 0.95))
}

func (latency *NetworkLatency) RTT() time.Duration {
	latency.mu.RLock()
	defer latency.mu.RUnlock()

	return latency.lastRTT
}

func (latency *NetworkLatency) MeanRTT() time.Duration {
	return latency.RTT()
}

func (latency *NetworkLatency) P95RTT() time.Duration {
	latency.mu.RLock()
	defer latency.mu.RUnlock()

	return latency.p95RTT
}

func (latency *NetworkLatency) OneWay() time.Duration {
	latency.mu.RLock()
	p95RTT := latency.p95RTT
	meanRTT := latency.lastRTT
	latency.mu.RUnlock()

	if p95RTT > 0 {
		return p95RTT / 2
	}

	if meanRTT > 0 {
		return meanRTT / 2
	}

	return fallbackOneWayLatency()
}

func (latency *NetworkLatency) Measured() bool {
	latency.mu.RLock()
	defer latency.mu.RUnlock()

	return latency.samples > 0
}

func fallbackOneWayLatency() time.Duration {
	config := viper.GetViper()

	if configured := config.GetDuration("trading.paper.default_one_way_latency"); configured > 0 {
		return configured
	}

	if pace := config.GetDuration("market.subscribe_pace"); pace > 0 {
		return pace
	}

	return 0
}

func PingIntervalFromViper() time.Duration {
	config := viper.GetViper()
	interval := config.GetDuration("market.ws_ping_interval")

	if interval <= 0 {
		interval = 15 * time.Second
	}

	return interval
}

package public

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/viper"
)

/*
LatencyProfile is the persisted Kraken websocket latency snapshot used by replay
and optimizer scoring without sleeping through real network delay.
*/
type LatencyProfile struct {
	RTTNS     int64     `json:"rtt_ns"`
	OneWayNS  int64     `json:"one_way_ns"`
	Samples   uint64    `json:"samples"`
	UpdatedAt time.Time `json:"updated_at"`
}

func LatencyProfilePath() string {
	config := viper.GetViper()
	path := config.GetString("trading.paper.latency_profile")

	if path != "" {
		return path
	}

	return "runs/network_latency.json"
}

func (profile LatencyProfile) RTT() time.Duration {
	if profile.RTTNS <= 0 {
		return 0
	}

	return time.Duration(profile.RTTNS)
}

func (profile LatencyProfile) OneWay() time.Duration {
	if profile.OneWayNS <= 0 {
		return 0
	}

	return time.Duration(profile.OneWayNS)
}

func LoadLatencyProfile() (LatencyProfile, bool) {
	path := LatencyProfilePath()
	payload, err := os.ReadFile(path)

	if err != nil {
		return LatencyProfile{}, false
	}

	var profile LatencyProfile

	if err := json.Unmarshal(payload, &profile); err != nil {
		return LatencyProfile{}, false
	}

	if profile.RTT() <= 0 {
		return LatencyProfile{}, false
	}

	return profile, true
}

func SaveLatencyProfile(profile LatencyProfile) error {
	path := LatencyProfilePath()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	payload, err := json.Marshal(profile)

	if err != nil {
		return err
	}

	return os.WriteFile(path, payload, 0o644)
}

var profilePersistMu sync.Mutex

func (latency *NetworkLatency) persistProfileLocked() {
	profile := LatencyProfile{
		RTTNS:     latency.lastRTT.Nanoseconds(),
		OneWayNS:  (latency.lastRTT / 2).Nanoseconds(),
		Samples:   latency.samples,
		UpdatedAt: latency.updated,
	}

	profilePersistMu.Lock()
	defer profilePersistMu.Unlock()

	_ = SaveLatencyProfile(profile)
}

func (latency *NetworkLatency) SnapshotProfile() LatencyProfile {
	latency.mu.RLock()
	defer latency.mu.RUnlock()

	return LatencyProfile{
		RTTNS:     latency.lastRTT.Nanoseconds(),
		OneWayNS:  (latency.lastRTT / 2).Nanoseconds(),
		Samples:   latency.samples,
		UpdatedAt: latency.updated,
	}
}

/*
ExchangeRoundTrip is the full client↔Kraken websocket latency for one request.
*/
func (latency *NetworkLatency) ExchangeRoundTrip() time.Duration {
	rtt := latency.RTT()

	if rtt > 0 {
		return rtt
	}

	oneWay := latency.OneWay()

	if oneWay <= 0 {
		return 0
	}

	return oneWay * 2
}

func ReplayExchangeLatency() time.Duration {
	if profile, ok := LoadLatencyProfile(); ok {
		return profile.RTT()
	}

	return 0
}

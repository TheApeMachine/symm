package paper

import (
	"bufio"
	"container/ring"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

func (ws *WebSocket) emulateLatency() {
	latency := ws.latencies.Value.(time.Duration)
	ws.latencies = ws.latencies.Next()
	time.Sleep(latency)
}

func (ws *WebSocket) loadLatencyProfile() (*ring.Ring, error) {
	profilePath := viper.GetString("trading.paper.latency_profile")

	if profilePath == "" {
		profilePath = "runs/network_latency.json"
	}

	profileBytes, err := os.ReadFile(profilePath)

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultLatencyRing(), nil
		}

		return nil, errnie.Error(err)
	}

	samples := make([]time.Duration, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(profileBytes)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		nanoseconds, parseErr := strconv.ParseInt(line, 10, 64)

		if errnie.Error(parseErr) != nil || nanoseconds <= 0 {
			return nil, errnie.Error(errors.New("paper websocket: invalid latency profile"))
		}

		samples = append(samples, time.Duration(nanoseconds))
	}

	if errnie.Error(scanner.Err()) != nil {
		return nil, scanner.Err()
	}

	if len(samples) == 0 {
		return nil, errnie.Error(errors.New("paper websocket: empty latency profile"))
	}

	latencyRing := ring.New(len(samples))

	for _, sample := range samples {
		latencyRing.Value = sample
		latencyRing = latencyRing.Next()
	}

	return latencyRing, nil
}

func defaultLatencyRing() *ring.Ring {
	latencyRing := ring.New(8)
	defaultLatency := viper.GetDuration("trading.paper.default_latency")

	if defaultLatency <= 0 {
		defaultLatency = 25 * time.Millisecond
	}

	for range 8 {
		latencyRing.Value = defaultLatency
		latencyRing = latencyRing.Next()
	}

	return latencyRing
}

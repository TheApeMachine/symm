package response

import (
	"container/ring"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
)

/*
Latency samples paper execution delay from the configured latency profile.
*/
type Latency struct {
	err     error
	timings *ring.Ring
}

/*
NewLatency constructs the paper execution delay sampler.
*/
func NewLatency() *Latency {
	return &Latency{}
}

/*
Error returns the latency sampler's terminal error.
*/
func (latency *Latency) Error() error {
	return latency.err
}

func (latency *Latency) Wait() {
	if latency == nil || latency.timings == nil {
		return
	}

	delay := latency.timings.Value.(time.Duration)
	latency.timings = latency.timings.Next()
	time.Sleep(delay)
}

/*
Load loads the latency profile from a file.
JSON profiles use {"latencies":[22,28,...]}; line-based profiles use one
positive millisecond sample per non-empty line.
*/
func (latency *Latency) Load(path string) *Latency {
	if strings.TrimSpace(path) == "" {
		latency.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"paper latency: profile path required",
			nil,
		))

		return latency
	}

	payload, err := os.ReadFile(path)

	if err != nil {
		latency.err = errnie.Error(errnie.Err(
			errnie.IO,
			"paper latency: profile unreadable",
			err,
		))

		return latency
	}

	timings, loadErr := latencySamples(payload)

	if loadErr != nil {
		latency.err = errnie.Error(loadErr)

		return latency
	}

	if len(timings) == 0 {
		latency.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"paper latency: profile has no samples",
			nil,
		))

		return latency
	}

	latency.timings = ring.New(len(timings))

	for _, timing := range timings {
		latency.timings.Value = timing
		latency.timings = latency.timings.Next()
	}

	return latency
}

func latencySamples(payload []byte) ([]time.Duration, error) {
	var profile struct {
		Latencies []int64 `json:"latencies"`
	}

	if json.Unmarshal(payload, &profile) == nil && len(profile.Latencies) > 0 {
		timings := make([]time.Duration, 0, len(profile.Latencies))

		for _, sample := range profile.Latencies {
			if sample <= 0 {
				return nil, errnie.Err(
					errnie.Validation,
					"paper latency: profile sample non-positive",
					nil,
				)
			}

			timings = append(timings, time.Duration(sample)*time.Millisecond)
		}

		return timings, nil
	}

	timings := make([]time.Duration, 0)

	for _, line := range strings.Split(string(payload), "\n") {
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		converted, parseErr := strconv.ParseInt(line, 10, 64)

		if parseErr != nil || converted <= 0 {
			return nil, errnie.Err(
				errnie.Validation,
				"paper latency: profile sample non-positive",
				parseErr,
			)
		}

		timings = append(timings, time.Duration(converted)*time.Millisecond)
	}

	return timings, nil
}

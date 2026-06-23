package response

import (
	"container/ring"
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
Each non-empty line must be a positive integer latency in milliseconds.
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

	timings := make([]time.Duration, 0)

	for _, line := range strings.Split(string(payload), "\n") {
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		converted, parseErr := strconv.ParseInt(line, 10, 64)

		if parseErr != nil || converted <= 0 {
			latency.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"paper latency: profile sample non-positive",
				parseErr,
			))

			return latency
		}

		timings = append(timings, time.Duration(converted)*time.Millisecond)
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

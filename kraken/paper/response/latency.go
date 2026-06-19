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
	delay := latency.timings.Value.(time.Duration)
	latency.timings = latency.timings.Next()
	time.Sleep(delay)
}

/*
Load loads the latency profile from a file.
*/
func (latency *Latency) Load(path string) *Latency {
	payload := errnie.Does(func() ([]byte, error) {
		return os.ReadFile(path)
	}).Or(func(err error) {
		latency.err = errnie.Error(errnie.Err(
			errnie.IO,
			"paper latency: profile unreadable",
			err,
		))
	}).Value()

	timings := make([]time.Duration, 0)
	// Loop over each line in the file and add it to a slice
	for _, line := range strings.Split(string(payload), "\n") {
		converted := errnie.Does(func() (int64, error) {
			return strconv.ParseInt(line, 10, 64)
		}).Or(func(err error) {
			latency.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"paper latency: profile sample non-positive",
				err,
			))
		}).Value()

		timings = append(timings, time.Duration(converted)*time.Millisecond)

		latency.timings = ring.New(len(timings))

		for _, timing := range timings {
			latency.timings.Value = timing
			latency.timings = latency.timings.Next()
		}
	}

	return latency
}

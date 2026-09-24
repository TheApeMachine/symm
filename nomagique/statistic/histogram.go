package statistic

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/theapemachine/errnie"
)

/*
HistogramServer bins a set of values with a Freedman-Diaconis width.
*/
type HistogramServer struct {
	out []byte
}

func NewHistogram() *HistogramServer {
	return &HistogramServer{}
}

/*
bin is one bucket of a histogram.
*/
type bin struct {
	ID    string  `json:"id"`
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
	Count int     `json:"count"`
}

func (server *HistogramServer) Write(ctx context.Context, call Histogram_write) error {
	server.out = nil
	payload, err := call.Args().Values()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.histogram: failed to read values", err))
	}

	if len(payload) == 0 {
		return nil
	}

	var values []float64

	if err := json.Unmarshal(payload, &values); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.histogram: values are not a JSON array of numbers", err))
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	if len(sorted) < 4 {
		return nil
	}

	quartile := func(fraction float64) float64 {
		position := fraction * float64(len(sorted)-1)
		lower := int(math.Floor(position))
		upper := int(math.Ceil(position))
		return sorted[lower] + (sorted[upper]-sorted[lower])*(position-float64(lower))
	}

	spread := quartile(0.75) - quartile(0.25)
	width := 2 * spread / math.Cbrt(float64(len(sorted)))

	if width <= 0 {
		return nil
	}

	minimum, maximum := sorted[0], sorted[len(sorted)-1]
	count := int(math.Floor((maximum-minimum)/width)) + 1
	bins := make([]bin, count)

	for index := range bins {
		lower := minimum + float64(index)*width
		bins[index] = bin{ID: fmt.Sprint(index), Lower: lower, Upper: lower + width}
	}

	for _, value := range sorted {
		index := min(int(math.Floor((value-minimum)/width)), count-1)
		bins[index].Count++
	}

	encoded, err := json.Marshal(bins)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.histogram: failed to encode bins", err))
	}

	server.out = encoded
	return nil
}

func (server *HistogramServer) Done(ctx context.Context, call Histogram_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.histogram: failed to allocate results", err))
	}

	results.SetIdle()

	if server.out != nil {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "statistic.histogram: failed to set out", err))
		}
	}

	server.out = nil
	return nil
}

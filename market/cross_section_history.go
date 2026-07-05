package market

import (
	"math"
	"time"

	"github.com/theapemachine/nomagique/correlation"
)

/*
CrossSectionHistory keeps rolling timestamped prices and derived returns.
It exists so CrossSection can reason about the universe without owning buffer mechanics.
*/
type CrossSectionHistory struct {
	Samples []correlation.Sample
	Returns []float64
}

func NewCrossSectionHistory() CrossSectionHistory {
	return CrossSectionHistory{
		Samples: []correlation.Sample{},
		Returns: []float64{},
	}
}

/*
Observe records one chronological price sample and its return.
*/
func (history *CrossSectionHistory) Observe(
	price float64,
	at time.Time,
	capacity int,
) {
	if len(history.Samples) > 0 {
		previous := history.Samples[len(history.Samples)-1]
		ret := math.Log(price / previous.Value)

		if ret != 0 || len(history.Returns) == 0 {
			history.pushReturn(ret, capacity)
		}
	}

	history.pushSample(correlation.Sample{At: at, Value: price}, capacity+1)
}

func (history *CrossSectionHistory) ReturnWindow(window int) []float64 {
	if len(history.Returns) == 0 || window <= 0 {
		return nil
	}

	if len(history.Returns) <= window {
		return append([]float64(nil), history.Returns...)
	}

	return append([]float64(nil), history.Returns[len(history.Returns)-window:]...)
}

func (history *CrossSectionHistory) SampleWindow(window int) []correlation.Sample {
	if len(history.Samples) == 0 || window <= 0 {
		return nil
	}

	if len(history.Samples) <= window {
		return append([]correlation.Sample(nil), history.Samples...)
	}

	return append([]correlation.Sample(nil), history.Samples[len(history.Samples)-window:]...)
}

func (history *CrossSectionHistory) pushReturn(value float64, capacity int) {
	if capacity < 1 {
		capacity = 1
	}

	history.Returns = append(history.Returns, value)

	if len(history.Returns) > capacity {
		history.Returns = history.Returns[len(history.Returns)-capacity:]
	}
}

func (history *CrossSectionHistory) pushSample(
	value correlation.Sample,
	capacity int,
) {
	if capacity < 1 {
		capacity = 1
	}

	history.Samples = append(history.Samples, value)

	if len(history.Samples) > capacity {
		history.Samples = history.Samples[len(history.Samples)-capacity:]
	}
}

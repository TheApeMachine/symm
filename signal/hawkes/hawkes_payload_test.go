package hawkes

import (
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/nomagique/algorithm"
)

func excitationBurstSamples(base time.Time, count int) []float64 {
	buyTimes := make([]float64, 0, count/2)
	sellTimes := make([]float64, 0, count/2)

	for index := range count {
		wall := base.Add(time.Duration(index) * 100 * time.Millisecond)
		seconds := float64(wall.UnixNano()) / float64(time.Second)

		if index%2 == 0 {
			sellTimes = append(sellTimes, seconds)
			continue
		}

		buyTimes = append(buyTimes, seconds)
	}

	horizon := float64(base.Add(time.Duration(count)*100*time.Millisecond).UnixNano()) / float64(time.Second)
	span := base.Add(time.Duration(count) * 100 * time.Millisecond).Sub(base)
	cooldown := algorithm.DeriveFitCooldown(span).Seconds()

	samples := []float64{
		horizon,
		cooldown,
		float64(len(buyTimes)),
		float64(len(sellTimes)),
	}
	samples = append(samples, buyTimes...)
	samples = append(samples, sellTimes...)

	return samples
}

func customExcitationPayload(
	base time.Time,
	buyOffsets, sellOffsets []time.Duration,
) []float64 {
	buyTimes := make([]float64, len(buyOffsets))
	sellTimes := make([]float64, len(sellOffsets))
	last := base

	for index, offset := range buyOffsets {
		wall := base.Add(offset)
		buyTimes[index] = float64(wall.UnixNano()) / float64(time.Second)

		if wall.After(last) {
			last = wall
		}
	}

	for index, offset := range sellOffsets {
		wall := base.Add(offset)
		sellTimes[index] = float64(wall.UnixNano()) / float64(time.Second)

		if wall.After(last) {
			last = wall
		}
	}

	if last.Equal(base) {
		last = base.Add(time.Millisecond)
	}

	span := last.Sub(base)
	if span <= 0 {
		span = time.Millisecond
	}

	return append(
		[]float64{
			float64(last.UnixNano()) / float64(time.Second),
			algorithm.DeriveFitCooldown(span).Seconds(),
			float64(len(buyTimes)),
			float64(len(sellTimes)),
		},
		append(buyTimes, sellTimes...)...,
	)
}

func warmExcitationScope(
	excitation *algorithm.Excitation,
	scope string,
	rows ...[]float64,
) {
	for _, row := range rows {
		processed := datura.Acquire("hawkes", datura.APPJSON)
		_ = processed.WithPayload(encodeFloatPayload(row...))
		processed.WithScope(scope)
		_ = transport.NewFlipFlop(processed, excitation)
		processed.Release()
	}
}

func frenzyExcitationPayload() []float64 {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	return excitationBurstSamples(base, 8)
}

func organicExcitationPayload() []float64 {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	return excitationBurstSamples(base, 128)
}

func shiftedOrganicPayload(offset time.Duration) []float64 {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).Add(offset)

	return excitationBurstSamples(base, 128)
}

func saturationGateWarmPayload() []float64 {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	buyOffsets := make([]time.Duration, 32)

	for index := range 32 {
		buyOffsets[index] = time.Duration(index) * 4 * time.Second
	}

	return customExcitationPayload(base, buyOffsets, nil)
}

func saturationBurstPayload() []float64 {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	return excitationBurstSamples(base.Add(10*time.Minute), 53)
}

func exhaustionFadePayload() []float64 {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	buyOffsets := make([]time.Duration, 32)

	for index := range 32 {
		buyOffsets[index] = time.Duration(index) * 4 * time.Second
	}

	return customExcitationPayload(base.Add(20*time.Minute), buyOffsets, nil)
}

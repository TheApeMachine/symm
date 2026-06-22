package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
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
		0,
	}
	samples = append(samples, buyTimes...)
	samples = append(samples, sellTimes...)

	return samples
}

func warmExcitationScope(
	excitation *algorithm.Excitation,
	scope string,
	rows ...[]float64,
) {
	for _, row := range rows {
		processed := datura.Acquire("hawkes", datura.APPJSON)
		processed.WithScope(scope)
		processed.WithPayload(equation.MarshalFeatureSchema(algorithm.ExcitationSampleInputKeys, row))
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

func TestExcitationPayloadWarmScope(testingTB *testing.T) {
	Convey("Given warmed excitation payloads", testingTB, func() {
		excitation := algorithm.NewExcitation(
			datura.Acquire("excitation-config", datura.APPJSON),
		)
		warmExcitationScope(
			excitation,
			"BTC/USD",
			frenzyExcitationPayload(),
			organicExcitationPayload(),
		)

		Convey("It should publish thermal strength", func() {
			So(excitation.Outcome().Strength, ShouldBeGreaterThan, 0)
		})
	})
}

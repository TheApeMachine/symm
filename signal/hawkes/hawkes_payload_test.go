package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm"
)

func excitationBurstInput(
	symbol string,
	base time.Time,
	count int,
) algorithm.ExcitationInput {
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

	return algorithm.ExcitationInput{
		Symbol:             symbol,
		HorizonSeconds:     horizon,
		FitCooldownSeconds: algorithm.DeriveFitCooldown(span).Seconds(),
		BuySeconds:         buyTimes,
		SellSeconds:        sellTimes,
	}
}

func warmExcitationScope(
	excitation *algorithm.Excitation,
	inputs ...algorithm.ExcitationInput,
) (algorithm.ExcitationOutcome, bool, error) {
	outcome := algorithm.ExcitationOutcome{}
	ready := false
	var err error

	for _, input := range inputs {
		outcome, ready, err = excitation.Measure(input)

		if err != nil {
			return algorithm.ExcitationOutcome{}, false, err
		}
	}

	return outcome, ready, nil
}

func TestExcitationPayloadWarmScope(testingTB *testing.T) {
	Convey("Given warmed excitation inputs", testingTB, func() {
		excitation := algorithm.NewExcitation()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		outcome, ready, err := warmExcitationScope(
			excitation,
			excitationBurstInput("BTC/USD", base, 8),
			excitationBurstInput("BTC/USD", base, 128),
		)

		Convey("It should publish thermal strength", func() {
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(outcome.Strength, ShouldBeGreaterThan, 0)
		})
	})
}

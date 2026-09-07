package strategy

import (
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	markettest "github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/types"
)

func runTape(testingTB testing.TB, laps int) (*Agent, []hindsight.LearningEvent) {
	testingTB.Helper()
	events := []hindsight.LearningEvent{}
	agent, books := agentFixture(testingTB, func(event hindsight.LearningEvent) error {
		events = append(events, event)
		return nil
	})
	books.current = spotbook.New()
	books.current.NoBookCrossing = false
	measurement := data.NewMeasurement[float64]("", "TEST/USD", "source", time.Time{}, time.Time{})
	ordinal := 0

	for range laps {
		tape := markettest.NewLevel3ChurnTape("TEST/USD", time.Unix(100, 0), 64)

		for _, message := range tape.Messages {
			books.update(message)
			ordinal++
			at := time.Unix(100, 0).Add(time.Duration(ordinal) * time.Second)
			agent.now = func() time.Time { return at }
			measurement.PutMetric(data.Metric[float64]{Label: "ordinal", Raw: float64(ordinal % 7)})

			if err := agent.Grid.Step([]*data.Measurement[float64]{measurement}); err != nil {
				testingTB.Fatal(err)
			}
			envelope := types.NewEnvelope(types.EnvelopeLevel3)
			envelope.Level3Data = kraken.Level3Data{Symbol: message.Symbol, Timestamp: at}
			agent.Step(envelope)

			if err := agent.Error(); err != nil {
				testingTB.Fatal(err)
			}
		}
	}
	return agent, events
}

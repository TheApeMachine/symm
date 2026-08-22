package audit

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

type mockPriceProvider struct {
	price float64
}

func (m *mockPriceProvider) Mark(symbol string, direction string) float64 {
	return m.price
}

type mockObserverFeedback struct {
	received  []types.MarkFeedback
	hindsight []types.HindsightFeedback
}

func (m *mockObserverFeedback) ObserveMark(feedback types.MarkFeedback) error {
	m.received = append(m.received, feedback)
	return nil
}

func (m *mockObserverFeedback) ObserveHindsight(feedback types.HindsightFeedback) error {
	m.hindsight = append(m.hindsight, feedback)
	return nil
}


func TestConcurrentObserver(t *testing.T) {
	Convey("Given a ConcurrentObserver monitoring staged decisions", t, func() {
		stager := NewStager(nil)
		provider := &mockPriceProvider{price: 105.0}
		feedback := &mockObserverFeedback{}
		observer := NewConcurrentObserver(stager, provider, feedback)
		observer.thresholdPct = 0.02

		decPrice, _ := decimal.NewFromString("100.0")
		decision := &types.Decision{
			ID:     "dec-1",
			Symbol: "BTC/USD",
			Action: types.ActionNothing,
			Mark:   decPrice,
		}

		stager.Stage(decision, 5*time.Millisecond)
		time.Sleep(10 * time.Millisecond)

		observer.evaluateMatured()

		Convey("A 5% excursion should trigger feedback to regulator", func() {
			So(feedback.received, ShouldHaveLength, 1)
			So(feedback.received[0].Symbol, ShouldEqual, "BTC/USD")
			So(feedback.received[0].Mark, ShouldEqual, 105.0)
			So(feedback.hindsight, ShouldHaveLength, 1)
			So(feedback.hindsight[0].Symbol, ShouldEqual, "BTC/USD")
			So(feedback.hindsight[0].Missed, ShouldBeTrue)
			So(feedback.hindsight[0].MissedReturn, ShouldAlmostEqual, 0.05, 1e-6)
		})
	})
}


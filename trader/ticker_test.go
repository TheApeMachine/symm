package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

type recordingSignal[T any] struct {
	rows []T
}

func (signal *recordingSignal[T]) IngestRoles() []string {
	return nil
}

func (signal *recordingSignal[T]) Categories() []types.CategoryType {
	return nil
}

func (signal *recordingSignal[T]) Measure(
	row T,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	signal.rows = append(signal.rows, row)
	return []*types.Measurement{{}}, nil
}

type benchmarkSignal[T any] struct{}

func (signal *benchmarkSignal[T]) IngestRoles() []string {
	return nil
}

func (signal *benchmarkSignal[T]) Categories() []types.CategoryType {
	return nil
}

func (signal *benchmarkSignal[T]) Measure(
	_ T,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	return []*types.Measurement{{}}, nil
}

func TestTickerMeasure(testingTB *testing.T) {
	Convey("Given a ticker with a typed signal", testingTB, func() {
		recording := &recordingSignal[kraken.TickerData]{}
		ticker := NewTicker([]types.Signal[kraken.TickerData]{recording})
		message := kraken.TickerDataSlice{{
			Symbol:    "BTC/USD",
			Bid:       99,
			Ask:       101,
			Last:      100,
			Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		}}

		Convey("When ticker data is measured", func() {
			measurements, err := ticker.Measure(message)

			Convey("It should measure each row through the signal", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldHaveLength, 1)
				So(recording.rows, ShouldHaveLength, 1)
				So(recording.rows[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}

func BenchmarkTickerMeasure(benchmarkTB *testing.B) {
	ticker := NewTicker([]types.Signal[kraken.TickerData]{
		&benchmarkSignal[kraken.TickerData]{},
	})
	message := kraken.TickerDataSlice{{
		Symbol:    "BTC/USD",
		Bid:       99,
		Ask:       101,
		Last:      100,
		Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
	}}

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		if _, err := ticker.Measure(message); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}

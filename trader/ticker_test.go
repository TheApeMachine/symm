package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

type recordingSignal struct {
	rows         []any
	crossSection *types.CrossSection
}

func (signal *recordingSignal) IngestRoles() []string {
	return nil
}

func (signal *recordingSignal) Categories() []types.CategoryType {
	return nil
}

func (signal *recordingSignal) Measure(
	row any,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	signal.rows = append(signal.rows, row)
	signal.crossSection = crossSection
	return []*types.Measurement{{}}, nil
}

type benchmarkSignal struct{}

func (signal *benchmarkSignal) IngestRoles() []string {
	return nil
}

func (signal *benchmarkSignal) Categories() []types.CategoryType {
	return nil
}

func (signal *benchmarkSignal) Measure(
	_ any,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	return []*types.Measurement{{}}, nil
}

func TestTickerMeasure(testingTB *testing.T) {
	Convey("Given a ticker with a typed signal", testingTB, func() {
		recording := &recordingSignal{}
		crossSection, crossSectionErr := types.NewCrossSection(
			types.DefaultCrossSectionConfig(),
		)
		ticker := NewTicker([]types.Signal[any]{recording}, crossSection)
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
				So(crossSectionErr, ShouldBeNil)
				So(err, ShouldBeNil)
				So(measurements, ShouldHaveLength, 1)
				So(recording.rows, ShouldHaveLength, 1)
				row := recording.rows[0].(kraken.TickerData)
				So(row.Symbol, ShouldEqual, "BTC/USD")
				So(recording.crossSection.Symbols(), ShouldResemble, []string{"BTC/USD"})
			})
		})
	})
}

func BenchmarkTickerMeasure(benchmarkTB *testing.B) {
	crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
	if err != nil {
		benchmarkTB.Fatal(err)
	}

	ticker := NewTicker([]types.Signal[any]{
		&benchmarkSignal{},
	}, crossSection)
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

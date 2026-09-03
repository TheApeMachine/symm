package relation

import (
	"slices"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestCompareCoordinate(t *testing.T) {
	Convey("Given a set of coordinates spanning every identity field", t, func() {
		coordinates := []Coordinate{
			{Symbol: "TEST/USD", Source: "cvd", Metric: "signed_net_fraction", Side: "buy", Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescalePerSecond, Epoch: 1},
			{Symbol: "TEST/USD", Source: "cvd", Metric: "signed_net_fraction", Side: "buy", Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescalePerSecond, Epoch: 2},
			{Symbol: "TEST/USD", Source: "cvd", Metric: "signed_net_fraction", Side: "sell", Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescalePerSecond, Epoch: 1},
			{Symbol: "TEST/USD", Source: "cvd", Metric: "signed_net_fraction", Side: "buy", Peer: "ALT/USD", Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescalePerSecond, Epoch: 1},
			{Symbol: "TEST/USD", Source: "hawkes", Metric: "arrival_rate", Unit: nmtypes.UnitEventsPerSecond, Timescale: nmtypes.TimescalePerMillisecond, Epoch: 1},
			{Symbol: "ALT/USD", Source: "cvd", Metric: "midpoint_log_return", Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescalePerSecond, Epoch: 1},
		}

		Convey("every coordinate equals itself", func() {
			for _, coordinate := range coordinates {
				So(CompareCoordinate(coordinate, coordinate), ShouldEqual, 0)
			}
		})

		Convey("comparison is antisymmetric for distinct coordinates", func() {
			for left := range coordinates {
				for right := range coordinates {
					So(CompareCoordinate(coordinates[left], coordinates[right]), ShouldEqual,
						-CompareCoordinate(coordinates[right], coordinates[left]))
				}
			}
		})

		Convey("comparison is transitive", func() {
			for first := range coordinates {
				for second := range coordinates {
					for third := range coordinates {
						if CompareCoordinate(coordinates[first], coordinates[second]) < 0 &&
							CompareCoordinate(coordinates[second], coordinates[third]) < 0 {
							So(CompareCoordinate(coordinates[first], coordinates[third]) < 0, ShouldBeTrue)
						}
					}
				}
			}
		})
	})

	Convey("Given coordinates whose rendered fields carry no separator or padding", t, func() {
		// Symbol/Source/Metric/Side/Peer contain no '/', and Epoch stays a
		// single digit, so the rendered ID joins fields in exactly the order
		// CompareCoordinate walks: lexicographic identity order must then
		// agree with the field-wise order.
		coordinates := []Coordinate{
			{Symbol: "A", Source: "cvd", Metric: "metric", Side: "buy", Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescalePerSecond, Epoch: 1},
			{Symbol: "A", Source: "cvd", Metric: "metric", Side: "buy", Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescalePerSecond, Epoch: 2},
			{Symbol: "A", Source: "cvd", Metric: "metric", Side: "buy", Peer: "B", Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescalePerSecond, Epoch: 1},
			{Symbol: "A", Source: "cvd", Metric: "metric", Side: "sell", Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescalePerSecond, Epoch: 1},
			{Symbol: "A", Source: "hawkes", Metric: "other", Unit: nmtypes.UnitRate, Timescale: nmtypes.TimescalePerMillisecond, Epoch: 1},
			{Symbol: "B", Source: "cvd", Metric: "metric", Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescalePerSecond, Epoch: 1},
		}

		Convey("the field-wise sign agrees with the rendered identity order", func() {
			for left := range coordinates {
				for right := range coordinates {
					fieldSign := sign(CompareCoordinate(coordinates[left], coordinates[right]))
					renderedSign := sign(strings.Compare(coordinates[left].ID(), coordinates[right].ID()))

					So(fieldSign, ShouldEqual, renderedSign)
				}
			}
		})
	})
}

func sign(value int) int {
	if value < 0 {
		return -1
	}

	if value > 0 {
		return 1
	}

	return 0
}

var benchmarkCompareSink int

func BenchmarkCompareCoordinate(b *testing.B) {
	coordinates := []Coordinate{
		{Symbol: "TEST/USD", Source: "cvd", Metric: "signed_net_fraction", Side: "buy", Unit: nmtypes.UnitCount, Timescale: nmtypes.TimescalePerSecond, Epoch: 1},
		{Symbol: "TEST/USD", Source: "hawkes", Metric: "arrival_rate", Unit: nmtypes.UnitEventsPerSecond, Timescale: nmtypes.TimescalePerMillisecond, Epoch: 1},
		{Symbol: "ALT/USD", Source: "cvd", Metric: "midpoint_log_return", Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescalePerSecond, Epoch: 2},
	}

	b.ReportAllocs()

	for iteration := 0; b.Loop(); iteration++ {
		left := coordinates[iteration%len(coordinates)]
		right := coordinates[(iteration+1)%len(coordinates)]
		benchmarkCompareSink += CompareCoordinate(left, right)
	}
}

func BenchmarkSortCoordinates(b *testing.B) {
	coordinates := make([]Coordinate, 256)

	for index := range coordinates {
		coordinates[index] = Coordinate{
			Symbol:    "TEST/USD",
			Source:    "cvd",
			Metric:    "metric",
			Side:      "buy",
			Unit:      nmtypes.UnitCount,
			Timescale: nmtypes.TimescalePerSecond,
			Epoch:     uint64(index % 4),
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		slices.SortFunc(coordinates, CompareCoordinate)
		benchmarkCompareSink += len(coordinates)
	}
}

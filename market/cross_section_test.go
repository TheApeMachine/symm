package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCrossSectionAggregateCache(testingTB *testing.T) {
	Convey("Given a warmed cross section", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		for index, name := range []string{"BTC/USD", "ETH/USD", "SOL/USD"} {
			row := &Symbol{
				Name:    name,
				Price:   100 + float64(index),
				Volume:  float64(1000 + index),
				Value:   0.01,
				Updated: base.Add(time.Duration(index) * time.Minute),
			}

			So(crossSection.Observe(row), ShouldBeNil)
		}

		Convey("It should serve breadth and volumes from cached aggregates", func() {
			So(crossSection.Breadth(time.Time{}), ShouldAlmostEqual, 1, 1e-9)
			So(len(crossSection.Volumes()), ShouldEqual, 3)
			So(crossSection.IsLeader("BTC/USD", 0.05, time.Time{}), ShouldBeTrue)
		})
	})
}

func TestCrossSectionLeader(testingTB *testing.T) {
	Convey("Given a universe where one obscure pair moves hardest", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		// BTC barely moves; UNFI rips. The leader must be the live mover, not
		// the largest-cap name.
		rows := []struct {
			name   string
			change float64
		}{
			{"BTC/USD", 0.01},
			{"ETH/USD", 0.012},
			{"SOL/USD", 0.008},
			{"UNFI/USD", 4.15},
		}

		for index, sample := range rows {
			row := &Symbol{
				Name:    sample.name,
				Price:   100 + float64(index),
				Volume:  1000,
				Value:   sample.change,
				Updated: base.Add(time.Duration(index) * time.Minute),
			}

			So(crossSection.Observe(row), ShouldBeNil)
		}

		Convey("It should anchor on the hardest mover, never a fixed major", func() {
			So(crossSection.Leader(), ShouldEqual, "UNFI/USD")
		})
	})
}

func TestCrossSectionLeaderEmptyWhenFlat(testingTB *testing.T) {
	Convey("Given a flat universe with no breakout", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		for index, name := range []string{"BTC/USD", "ETH/USD", "SOL/USD"} {
			row := &Symbol{
				Name:    name,
				Price:   100 + float64(index),
				Volume:  1000,
				Value:   0.01,
				Updated: base.Add(time.Duration(index) * time.Minute),
			}

			So(crossSection.Observe(row), ShouldBeNil)
		}

		Convey("It should report no leader rather than picking one by vibes", func() {
			So(crossSection.Leader(), ShouldEqual, "")
		})
	})
}

func TestPeerWindowSnapshotCache(testingTB *testing.T) {
	Convey("Given a warmed cross section", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		for index, name := range []string{"BTC/USD", "ETH/USD", "SOL/USD"} {
			price := 100.0 + float64(index)

			for step := range 5 {
				row := &Symbol{
					Name:    name,
					Price:   price * (1 + 0.01*float64(step)),
					Volume:  1000,
					Value:   0.01,
					Updated: base.Add(time.Duration(step) * time.Minute),
				}

				So(crossSection.Observe(row), ShouldBeNil)
			}
		}

		first := crossSection.PeerWindowSnapshot(3, time.Time{})
		second := crossSection.PeerWindowSnapshot(3, time.Time{})

		Convey("It should reuse the cached snapshot for the same window", func() {
			So(len(first.MarketReturns), ShouldEqual, len(second.MarketReturns))
			So(first.MarketReturns, ShouldResemble, second.MarketReturns)
		})
	})
}

func BenchmarkPeerWindowSnapshot(b *testing.B) {
	crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

	if err != nil {
		b.Fatal(err)
	}

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for index := range 32 {
		row := &Symbol{
			Name:    "SYM/USD",
			Price:   100 + float64(index%5),
			Volume:  1000,
			Value:   0.01,
			Updated: base.Add(time.Duration(index) * time.Second),
		}

		_ = crossSection.Observe(row)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = crossSection.PeerWindowSnapshot(3, time.Time{})
	}
}

package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestCrossSectionAggregateCache(testingTB *testing.T) {
	Convey("Given a warmed cross section from ticker artifacts", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		for index, name := range []string{"BTC/USD", "ETH/USD", "SOL/USD"} {
			So(observeTicker(
				crossSection,
				base.Add(time.Duration(index)*time.Minute),
				tickerRow(name, 100+float64(index), 1000+float64(index), 0.01),
			), ShouldBeNil)
		}

		Convey("It should serve breadth and volumes from cached aggregates", func() {
			So(crossSection.Breadth(), ShouldAlmostEqual, 1, 1e-9)
			So(len(crossSection.Volumes()), ShouldEqual, 3)
			So(crossSection.IsLeader("BTC/USD", 0.05), ShouldBeTrue)
		})
	})
}

func TestCrossSectionLeader(testingTB *testing.T) {
	Convey("Given a universe where one obscure pair moves hardest", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
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
			So(observeTicker(
				crossSection,
				base.Add(time.Duration(index)*time.Minute),
				tickerRow(sample.name, 100+float64(index), 1000, sample.change),
			), ShouldBeNil)
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
			So(observeTicker(
				crossSection,
				base.Add(time.Duration(index)*time.Minute),
				tickerRow(name, 100+float64(index), 1000, 0.01),
			), ShouldBeNil)
		}

		Convey("It should report no leader rather than picking one by vibes", func() {
			So(crossSection.Leader(), ShouldEqual, "")
		})
	})
}

func TestPeerWindowSnapshotCache(testingTB *testing.T) {
	Convey("Given a warmed cross section from ticker artifacts", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		for index, name := range []string{"BTC/USD", "ETH/USD", "SOL/USD"} {
			price := 100.0 + float64(index)

			for step := range 5 {
				So(observeTicker(
					crossSection,
					base.Add(time.Duration(step)*time.Minute),
					tickerRow(name, price*(1+0.01*float64(step)), 1000, 0.01),
				), ShouldBeNil)
			}
		}

		first := crossSection.PeerCache.Snapshot(crossSection, 3)
		second := crossSection.PeerCache.Snapshot(crossSection, 3)

		Convey("It should reuse the cached snapshot for the same window", func() {
			firstReturns := datura.Peek[[]float64](first, "market_returns")
			secondReturns := datura.Peek[[]float64](second, "market_returns")

			So(len(firstReturns), ShouldEqual, len(secondReturns))
			So(firstReturns, ShouldResemble, secondReturns)
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
		_ = observeTicker(
			crossSection,
			base.Add(time.Duration(index)*time.Second),
			tickerRow("SYM/USD", 100+float64(index%5), 1000, 0.01),
		)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = crossSection.PeerCache.Snapshot(crossSection, 3)
	}
}

func observeTicker(
	crossSection *CrossSection,
	at time.Time,
	rows ...map[string]any,
) error {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	defer artifact.Release()

	artifact.WithRole("ticker").
		WithScope("ticker").
		WithPayload(datura.Map[any]{
			"channel": "ticker",
			"type":    "update",
			"data":    rows,
		}.Marshal())
	artifact.SetTimestamp(at.UnixNano())

	return crossSection.Observe(map[string][]*datura.Artifact{
		"ticker": []*datura.Artifact{artifact},
	})
}

func tickerRow(
	symbol string,
	price float64,
	volume float64,
	change float64,
) map[string]any {
	return map[string]any{
		"symbol":     symbol,
		"last":       price,
		"volume":     volume,
		"change_pct": change * 100,
	}
}

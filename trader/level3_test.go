package trader

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

type level3AnalyzerStub struct {
	observed chan string
	advanced chan string
	rows     chan kraken.Level3Data
	results  chan manifold.ProcessResult
}

func (analyzer *level3AnalyzerStub) ObserveLevel3(
	row kraken.Level3Data,
	_ int,
	_ int,
	_ manifold.Level3Book,
) manifold.ProcessResult {
	analyzer.observed <- row.Symbol

	if analyzer.rows != nil {
		analyzer.rows <- row
	}

	if analyzer.results != nil {
		select {
		case result := <-analyzer.results:
			return result
		default:
		}
	}

	return manifold.ProcessResult{AdvanceReady: true}
}

func (analyzer *level3AnalyzerStub) AdvanceLevel3(symbol string) {
	analyzer.advanced <- symbol
}

type level3InstrumentStub struct {
	refreshed chan string
}

func (instrument *level3InstrumentStub) Pair(string) (kraken.InstrumentPair, error) {
	return kraken.InstrumentPair{PricePrecision: 1, QtyPrecision: 8}, nil
}

func (instrument *level3InstrumentStub) RefreshLevel3(symbol string) error {
	if instrument.refreshed != nil {
		instrument.refreshed <- symbol
	}

	return nil
}

type level3SignalStub struct{}

func (signal *level3SignalStub) IngestRoles() []string {
	return []string{channelTrade, channelLevel3}
}

type typedLevel3SignalStub struct{}

func (signal *typedLevel3SignalStub) IngestRoles() []string {
	return []string{channelLevel3}
}

func (signal *typedLevel3SignalStub) Measure(
	input any,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	row := input.(kraken.Level3Data)

	return []*types.Measurement{{
		Source:  types.SourceHawkes,
		Metric:  types.MetricEventCount,
		Subject: types.SubjectTradeArrivals,
		Stream:  channelLevel3,
		Symbol:  row.Symbol,
		At:      row.Timestamp,
		Raw:     1,
	}}, nil
}

func (signal *level3SignalStub) Measure(
	input any,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	measurement := &types.Measurement{Source: types.SourceToxicity}

	switch row := input.(type) {
	case kraken.TradeData:
		measurement.Symbol = row.Symbol
		measurement.Stream = "trades"
	case kraken.Level3Data:
		measurement.Symbol = row.Symbol
		measurement.Stream = channelLevel3
	}

	return []*types.Measurement{measurement}, nil
}

func newLevel3TestOwner(
	t *testing.T,
	analyzer *level3AnalyzerStub,
) *Level3 {
	t.Helper()
	viper.Set("market.l3_ring_capacity", 8)
	viper.Set("market.universe.trading_tier_size", 3)

	level3 := NewLevel3(
		context.Background(),
		&Signal{Level3: []types.Signal[any]{&level3SignalStub{}}},
		nil,
		&level3InstrumentStub{},
		analyzer,
		NewLevel3Book(10),
	)

	if level3 == nil {
		t.Fatal("NewLevel3 returned nil")
	}

	return level3
}

func receiveSymbol(t *testing.T, symbols <-chan string) string {
	t.Helper()

	select {
	case symbol := <-symbols:
		return symbol
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for level3 owner")
		return ""
	}
}

func TestNewLevel3(t *testing.T) {
	Convey("Given one Level3-owned signal with two ingest roles", t, func() {
		analyzer := &level3AnalyzerStub{
			observed: make(chan string, 1),
			advanced: make(chan string, 1),
		}
		level3 := newLevel3TestOwner(t, analyzer)
		defer level3.Close()

		Convey("It should derive one fixed mailbox identity per tier symbol and role", func() {
			So(level3.mailbox.slots, ShouldHaveLength, 6)
		})
	})

	Convey("Given an invalid configured L3 ingress capacity", t, func() {
		capacity := viper.GetInt("market.l3_ring_capacity")
		defer viper.Set("market.l3_ring_capacity", capacity)
		viper.Set("market.l3_ring_capacity", 3)
		analyzer := &level3AnalyzerStub{
			observed: make(chan string, 1),
			advanced: make(chan string, 1),
		}

		Convey("It should reject construction without a fallback", func() {
			level3 := NewLevel3(
				context.Background(),
				&Signal{Level3: []types.Signal[any]{&level3SignalStub{}}},
				nil,
				&level3InstrumentStub{},
				analyzer,
				NewLevel3Book(10),
			)
			So(level3, ShouldBeNil)
		})
	})
}

func TestLevel3Consume(t *testing.T) {
	Convey("Given independent producers physically publish sequence two before one", t, func() {
		analyzer := &level3AnalyzerStub{
			observed: make(chan string, 4),
			advanced: make(chan string, 4),
		}
		level3 := newLevel3TestOwner(t, analyzer)
		defer level3.Close()

		second := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"A","timestamp":"2026-07-12T00:00:02Z","checksum":2}]}`)
		first := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"B","timestamp":"2026-07-12T00:00:01Z","checksum":1}]}`)
		So(level3.ring.Push(level3Frame{sequence: 2, stream: channelLevel3, raw: second}), ShouldBeTrue)
		So(level3.ring.Push(level3Frame{sequence: 1, stream: channelLevel3, raw: first}), ShouldBeTrue)
		level3.wake <- struct{}{}

		Convey("It should observe every row and advance fairly in claimed order", func() {
			So(receiveSymbol(t, analyzer.observed), ShouldEqual, "B")
			So(receiveSymbol(t, analyzer.observed), ShouldEqual, "A")
			So(receiveSymbol(t, analyzer.advanced), ShouldEqual, "B")
			So(receiveSymbol(t, analyzer.advanced), ShouldEqual, "A")
			So(level3.Status(), ShouldEqual, types.READY)
		})
	})
}

func TestLevel3OnTrade(t *testing.T) {
	Convey("Given a new Level3 owner", t, func() {
		analyzer := &level3AnalyzerStub{
			observed: make(chan string, 1),
			advanced: make(chan string, 1),
		}
		level3 := newLevel3TestOwner(t, analyzer)
		defer level3.Close()

		Convey("When it observes only a trade frame", func() {
			level3.OnTrade([]byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":100,"qty":1,"timestamp":"2026-07-12T00:00:01Z"}]}`))
			deadline := time.Now().Add(5 * time.Second)
			measurements := make([]*types.Measurement, 0)

			for len(measurements) == 0 && time.Now().Before(deadline) {
				measurements, _ = level3.Measure()
			}

			Convey("It should run the owned signal without claiming L3 readiness", func() {
				So(measurements, ShouldHaveLength, 1)
				So(measurements[0].Stream, ShouldEqual, "trades")
				So(level3.Status(), ShouldEqual, types.INITIALIZING)
			})
		})
	})
}

func TestLevel3Measure(t *testing.T) {
	Convey("Given a typed measurement owned by the Level3 path", t, func() {
		level3 := &Level3{signals: []types.Signal[any]{&typedLevel3SignalStub{}}}
		row := kraken.Level3Data{
			Symbol:    "BTC/USD",
			Timestamp: time.Unix(1, 0),
		}

		Convey("When the compatibility price adapter receives it", func() {
			measurements := level3.measure(row, 100)

			Convey("Then the immutable typed record still reaches the mailbox path", func() {
				So(measurements, ShouldHaveLength, 1)
				So(measurements[0].Metric, ShouldEqual, types.MetricEventCount)
				So(measurements[0].Metrics, ShouldBeNil)
			})
		})
	})
}

func TestLevel3Close(t *testing.T) {
	Convey("Given a running Level3 owner", t, func() {
		analyzer := &level3AnalyzerStub{
			observed: make(chan string, 1),
			advanced: make(chan string, 1),
		}
		level3 := newLevel3TestOwner(t, analyzer)

		Convey("When Close cancels the owner context", func() {
			So(level3.Close(), ShouldBeNil)
			level3.On([]byte(`{"channel":"level3","data":[]}`))

			Convey("It should reject further publication by leaving ingress empty", func() {
				So(level3.ctx.Err(), ShouldEqual, context.Canceled)
				So(level3.ring.Len(), ShouldEqual, 0)
			})
		})
	})
}

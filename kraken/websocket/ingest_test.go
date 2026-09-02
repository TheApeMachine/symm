package websocket

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	derivativessignal "github.com/theapemachine/symm/signal/derivatives"
	"github.com/theapemachine/symm/types"
)

var manifestBenchmark hindsight.EnvelopeManifest
var futuresTradeEnvelopeBenchmark []*types.Envelope

func TestManifestFor(t *testing.T) {
	venueAt := time.Date(2026, time.September, 1, 21, 41, 5, 123, time.UTC)
	testCases := []struct {
		name     string
		envelope *types.Envelope
		expected time.Time
	}{
		{
			name: "ticker venue time",
			envelope: &types.Envelope{
				TypeID:     types.EnvelopeTicker,
				TickerData: kraken.TickerData{Timestamp: venueAt},
			},
			expected: venueAt,
		},
		{
			name: "trade venue time",
			envelope: &types.Envelope{
				TypeID:    types.EnvelopeTrade,
				TradeData: kraken.TradeData{Timestamp: venueAt},
			},
			expected: venueAt,
		},
		{
			name: "level3 venue time",
			envelope: &types.Envelope{
				TypeID:     types.EnvelopeLevel3,
				Level3Data: kraken.Level3Data{Timestamp: venueAt},
			},
			expected: venueAt,
		},
		{
			name: "execution venue time",
			envelope: &types.Envelope{
				TypeID:        types.EnvelopeExecution,
				ExecutionData: kraken.ExecutionData{Timestamp: venueAt},
			},
			expected: venueAt,
		},
		{
			name: "futures ticker venue time",
			envelope: &types.Envelope{
				TypeID:            types.EnvelopeFuturesTicker,
				FuturesTickerData: kraken.FuturesTickerData{Timestamp: venueAt},
			},
			expected: venueAt,
		},
		{
			name: "futures trade venue time",
			envelope: &types.Envelope{
				TypeID:           types.EnvelopeFuturesTrade,
				FuturesTradeData: kraken.FuturesTradeData{Timestamp: venueAt},
			},
			expected: venueAt,
		},
		{
			name: "synthetic futures ticker time",
			envelope: &types.Envelope{
				TypeID: types.EnvelopeFuturesTicker,
				FuturesTickerData: kraken.FuturesTickerData{
					Timestamp:          venueAt,
					SyntheticTimestamp: true,
				},
			},
		},
		{
			name: "synthetic futures trade time",
			envelope: &types.Envelope{
				TypeID: types.EnvelopeFuturesTrade,
				FuturesTradeData: kraken.FuturesTradeData{
					Timestamp:          venueAt,
					SyntheticTimestamp: true,
				},
			},
		},
		{
			name: "unsupported derived envelope",
			envelope: &types.Envelope{
				TypeID: types.EnvelopeCorrelation,
			},
		},
	}

	for _, testCase := range testCases {
		Convey("Given an envelope with "+testCase.name, t, func() {
			manifest := manifestFor(
				testCase.envelope,
				hindsight.CaptureIdentity{},
				0,
				"workload",
				"BTC/USD",
			)

			Convey("The manifest should preserve only protocol-supplied venue time", func() {
				So(manifest.VenueAt.Equal(testCase.expected), ShouldBeTrue)
			})
		})
	}
}

func TestFromFuturesTrade(t *testing.T) {
	Convey("Given the reverse-causal futures snapshot captured for LTC/USD", t, func() {
		newerAt := time.UnixMilli(1788303913066).UTC()
		olderAt := time.UnixMilli(1788303910195).UTC()
		trade := &kraken.FuturesTrade{
			Feed: "trade_snapshot",
			Data: []kraken.FuturesTradeData{
				{
					Symbol: "LTC/USD", Price: *decimal.NewFromFloat64(49.45),
					Qty: 0.14, Side: "buy", Type: "fill", UID: "newer", Timestamp: newerAt,
				},
				{
					Symbol: "LTC/USD", Price: *decimal.NewFromFloat64(49.44),
					Qty: 0.04, Side: "sell", Type: "fill", UID: "older", Timestamp: olderAt,
				},
			},
		}

		Convey("Its envelopes should become a causal stream before derivatives observes them", func() {
			envelopes, manifests := fromFuturesTrade(trade, hindsight.CaptureIdentity{})

			So(envelopes, ShouldHaveLength, 2)
			So(manifests, ShouldHaveLength, 2)
			So(envelopes[0].FuturesTradeData.UID, ShouldEqual, "older")
			So(envelopes[0].CaptureOrdinal, ShouldEqual, uint64(0))
			So(manifests[0].VenueAt.Equal(olderAt), ShouldBeTrue)
			So(envelopes[1].FuturesTradeData.UID, ShouldEqual, "newer")
			So(envelopes[1].CaptureOrdinal, ShouldEqual, uint64(1))
			So(manifests[1].VenueAt.Equal(newerAt), ShouldBeTrue)

			entity := derivativessignal.NewTrade()

			for _, envelope := range envelopes {
				measurement := entity.Step(envelope.FuturesTradeData)

				So(measurement.Err, ShouldBeNil)
				So(measurement.From.After(measurement.At), ShouldBeFalse)
			}
		})
	})

	Convey("Given a live futures trade frame", t, func() {
		trade := &kraken.FuturesTrade{
			Feed: "trade",
			Data: []kraken.FuturesTradeData{{UID: "live"}},
		}

		Convey("Its wire order should remain unchanged", func() {
			envelopes, _ := fromFuturesTrade(trade, hindsight.CaptureIdentity{})

			So(envelopes, ShouldHaveLength, 1)
			So(envelopes[0].FuturesTradeData.UID, ShouldEqual, "live")
		})
	})
}

func BenchmarkManifestFor(b *testing.B) {
	venueAt := time.Date(2026, time.September, 1, 21, 41, 5, 123, time.UTC)
	envelope := types.NewEnvelope(types.EnvelopeTicker)
	envelope.TickerData.Timestamp = venueAt

	for b.Loop() {
		manifestBenchmark = manifestFor(
			envelope,
			hindsight.CaptureIdentity{},
			0,
			"ticker",
			"BTC/USD",
		)
	}
}

func BenchmarkFromFuturesTrade(b *testing.B) {
	trade := &kraken.FuturesTrade{
		Feed: "trade_snapshot",
		Data: make([]kraken.FuturesTradeData, 100),
	}

	for index := range trade.Data {
		trade.Data[index].Symbol = "LTC/USD"
		trade.Data[index].Timestamp = time.UnixMilli(1788303913066 - int64(index)).UTC()
	}

	b.ReportAllocs()

	for b.Loop() {
		futuresTradeEnvelopeBenchmark, _ = fromFuturesTrade(
			trade,
			hindsight.CaptureIdentity{},
		)
	}
}

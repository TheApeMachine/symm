package catalog_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/tests/catalog"
	"github.com/theapemachine/symm/types"
)

/*
TestCatalogSignalMeasures proves each market-facing signal's catalog Kind through
stack.Boot + Crypto.Tick — not calm-vs-stress relative lifts.
*/
func TestCatalogSignalMeasures(t *testing.T) {
	Convey("Given catalog measure proofs for market-facing signals", t, func() {
		Convey("When the pump kind is measured with pumpdump only", func() {
			Convey("Then ProveMeasure accepts the pump ignition tape", func() {
				catalog.ProveMeasure(t, catalog.KindPump, pumpdumpOnly)
			})
		})

		Convey("When the coil kind is measured with pumpdump only", func() {
			Convey("Then ProveMeasure accepts the coil breakout tape", func() {
				catalog.ProveMeasure(t, catalog.KindCoil, pumpdumpOnly)
			})
		})

		Convey("When the exhaustion kind is measured with exhaust only", func() {
			Convey("Then ProveMeasure accepts the exhaustion reject tape", func() {
				catalog.ProveMeasure(t, catalog.KindExhaustion, exhaustOnly)
			})
		})

		Convey("When the vacuum kind is measured with exhaust only", func() {
			Convey("Then ProveMeasure accepts the liquidity vacuum tape", func() {
				catalog.ProveMeasure(t, catalog.KindVacuum, exhaustOnly)
			})
		})

		Convey("When the sector_lift kind is measured with correlation only", func() {
			Convey("Then ProveMeasure accepts the sector herd lift tape", func() {
				catalog.ProveMeasure(t, catalog.KindSectorLift, correlationOnly)
			})
		})

		Convey("When the thin_book kind is measured with liquidity only", func() {
			Convey("Then ProveMeasure accepts the thin book trap tape", func() {
				catalog.ProveMeasure(t, catalog.KindThinBook, liquidityOnly)
			})
		})

		Convey("When the noise kind is measured with sentiment only", func() {
			Convey("Then ProveMeasure accepts the noise no-herd tape", func() {
				catalog.ProveMeasure(t, catalog.KindNoise, sentimentOnly)
			})
		})

		Convey("When the toxic_chase kind is measured with cvd and hawkes", func() {
			Convey("Then ProveMeasure accepts the toxic aggression tapes", func() {
				catalog.ProveMeasure(t, catalog.KindToxicChase, toxicAndHawkes)
			})
		})

		Convey("When the lag_no_lead kind is measured with leadlag only", func() {
			Convey("Then ProveMeasure accepts the lag without lead tape", func() {
				catalog.ProveMeasure(t, catalog.KindLagNoLead, leadlagOnly)
			})
		})
	})
}

func pumpdumpOnly(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{pumpdump.NewSignal(ctx, api, channel)}
}

func exhaustOnly(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{exhaust.NewSignal(ctx, api, instrument, channel)}
}

func correlationOnly(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{correlation.NewSignal(ctx, api, channel)}
}

func liquidityOnly(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{liquidity.NewSignal(ctx, api, channel)}
}

func sentimentOnly(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{sentiment.NewSignal(ctx, api, channel)}
}

func leadlagOnly(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{leadlag.NewSignal(ctx, api, channel)}
}

func toxicAndHawkes(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{
		cvd.NewSignal(ctx, api, channel),
		hawkes.NewSignal(ctx, api, channel),
	}
}

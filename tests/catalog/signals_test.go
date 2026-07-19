package catalog_test

import (
	"context"
	"testing"

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
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/catalog"
	"github.com/theapemachine/symm/types"
)

/*
TestCatalogSignalMeasures proves each market-facing signal's catalog Kind through
stack.Boot + Crypto.Tick — not calm-vs-stress relative lifts.
*/
func TestCatalogSignalMeasures(t *testing.T) {
	cases := []struct {
		kind    catalog.ScenarioKind
		signals tests.SignalFactory
	}{
		{catalog.KindPump, pumpdumpOnly},
		{catalog.KindCoil, pumpdumpOnly},
		{catalog.KindExhaustion, exhaustOnly},
		{catalog.KindVacuum, exhaustOnly},
		{catalog.KindSectorLift, correlationOnly},
		{catalog.KindThinBook, liquidityOnly},
		{catalog.KindNoise, sentimentOnly},
		{catalog.KindToxicChase, toxicAndHawkes},
		{catalog.KindLagNoLead, leadlagOnly},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(string(testCase.kind), func(t *testing.T) {
			catalog.ProveMeasure(t, testCase.kind, testCase.signals)
		})
	}
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

package hawkes

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken/trade"
)

const testFixtureWindow = 5 * time.Minute

func burstEvents(start time.Time, count int, gap time.Duration) []time.Time {
	events := make([]time.Time, count)

	for index := range events {
		events[index] = start.Add(time.Duration(index) * gap)
	}

	return events
}

func sparseSellEvents(start time.Time, count int) []time.Time {
	sells := make([]time.Time, count)

	for index := range sells {
		sells[index] = start.Add(time.Duration(index+1) * time.Second)
	}

	return sells
}

func balancedBurstEvents(
	start time.Time,
	buyCount, sellCount int,
	buyGap time.Duration,
) ([]time.Time, []time.Time) {
	return burstEvents(start, buyCount, buyGap), sparseSellEvents(start.Add(-time.Second), sellCount)
}

func ticksFromSideEvents(buyEvents, sellEvents []time.Time) []trade.Data {
	ticks := make([]trade.Data, 0, len(buyEvents)+len(sellEvents))

	for _, eventTime := range buyEvents {
		ticks = append(ticks, trade.Data{Side: "buy", Timestamp: eventTime})
	}

	for _, eventTime := range sellEvents {
		ticks = append(ticks, trade.Data{Side: "sell", Timestamp: eventTime})
	}

	return ticks
}

func fitContextForEventCount(t *testing.T, count int) FitContext {
	t.Helper()

	start := time.Unix(0, 0)
	buyEvents := burstEvents(start, count/2+1, 40*time.Millisecond)
	sellEvents := burstEvents(start.Add(time.Millisecond), count/2, 45*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(100 * time.Millisecond)
	stream := NewArrivalStream(buyEvents, sellEvents)
	context, ok := NewFitContext(stream, horizon)

	if !ok {
		t.Fatalf("expected fit context for %d events", count)
	}

	return context
}

func fitForEventsFixture(t *testing.T) (time.Time, ArrivalStream) {
	t.Helper()

	start := time.Unix(10_000, 0)
	buyEvents := burstEvents(start, 16, 50*time.Millisecond)
	sellEvents := burstEvents(start.Add(5*time.Millisecond), 6, 80*time.Millisecond)
	now := buyEvents[len(buyEvents)-1].Add(10 * time.Millisecond)
	ticks := ticksFromSideEvents(buyEvents, sellEvents)
	_, stream, ok := FitContextFromTicks(ticks, time.Time{}, now)

	if !ok {
		t.Fatal("expected fit context from fixture")
	}

	return now, stream
}

func sampleFit() BivariateFit {
	return BivariateFit{
		MuBuy:          1,
		MuSell:         1,
		AlphaBB:        2,
		AlphaBS:        0.5,
		AlphaSB:        0.2,
		AlphaSS:        0.3,
		Beta:           4,
		BuyIntensity:   3,
		SellIntensity:  1.2,
		SpectralRadius: 0.5,
	}
}

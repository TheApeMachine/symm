package manifold

import (
	"strconv"
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken"
)

func BenchmarkPopulationApply(b *testing.B) {
	const ordersPerLevel = 2
	at := time.Unix(1, 0)
	snapshot := kraken.Level3Data{Type: "snapshot", Timestamp: at}

	for level := 0; level < testBookDepth; level++ {
		for orderIndex := 0; orderIndex < ordersPerLevel; orderIndex++ {
			suffix := strconv.Itoa(level) + "-" + strconv.Itoa(orderIndex)
			snapshot.Bids = append(snapshot.Bids, kraken.Level3Order{
				OrderID: "bid-" + suffix, LimitPrice: 100 - float64(level),
				OrderQty: 1, Timestamp: at,
			})
			snapshot.Asks = append(snapshot.Asks, kraken.Level3Order{
				OrderID: "ask-" + suffix, LimitPrice: 110 + float64(level),
				OrderQty: 1, Timestamp: at,
			})
		}
	}

	newBest := kraken.Level3Data{
		Type: "update", Timestamp: at,
		Bids: []kraken.Level3Order{
			{Event: "add", OrderID: "bid-new-0", LimitPrice: 101, OrderQty: 1, Timestamp: at},
			{Event: "add", OrderID: "bid-new-1", LimitPrice: 101, OrderQty: 1, Timestamp: at},
		},
		Asks: []kraken.Level3Order{
			{Event: "add", OrderID: "ask-new-0", LimitPrice: 109, OrderQty: 1, Timestamp: at},
			{Event: "add", OrderID: "ask-new-1", LimitPrice: 109, OrderQty: 1, Timestamp: at},
		},
	}
	republish := kraken.Level3Data{
		Type: "update", Timestamp: at,
		Bids: []kraken.Level3Order{
			{Event: "delete", OrderID: "bid-new-0", LimitPrice: 101, Timestamp: at},
			{Event: "delete", OrderID: "bid-new-1", LimitPrice: 101, Timestamp: at},
		},
		Asks: []kraken.Level3Order{
			{Event: "delete", OrderID: "ask-new-0", LimitPrice: 109, Timestamp: at},
			{Event: "delete", OrderID: "ask-new-1", LimitPrice: 109, Timestamp: at},
		},
	}

	for _, wire := range snapshot.Bids[len(snapshot.Bids)-ordersPerLevel:] {
		wire.Event = "add"
		republish.Bids = append(republish.Bids, wire)
	}

	for _, wire := range snapshot.Asks[len(snapshot.Asks)-ordersPerLevel:] {
		wire.Event = "add"
		republish.Asks = append(republish.Asks, wire)
	}

	population := NewPopulation("BTC/USD", nil, testBookDepth)
	population.Apply(snapshot)
	updates := []kraken.Level3Data{newBest, republish}
	updateIndex := 0
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		population.Apply(updates[updateIndex%len(updates)])
		updateIndex++
	}

	if !population.Ready() {
		b.Fatal(population.InvalidReason())
	}
}

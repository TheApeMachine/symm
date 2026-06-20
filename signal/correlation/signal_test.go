package correlation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
)

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func measurementQuery(scope string) *datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return acquired
}

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
}

func insertTreeArtifact(signal *Signal, role, scope string, payload []byte) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	if wire, err := artifact.Message().Marshal(); err == nil && len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

func insertTrades(signal *Signal, scope string, updates []tradeUpdate) {
	raw, err := json.Marshal(updates)

	if err != nil {
		panic(err)
	}

	insertTreeArtifact(signal, "trade", scope, raw)
}

func insertTradeRow(
	signal *Signal,
	symbol string,
	price, qty float64,
	eventAt time.Time,
) {
	insertTrades(signal, symbol, []tradeUpdate{{
		Symbol:    symbol,
		Price:     price,
		Qty:       qty,
		Timestamp: eventAt,
	}})
}

func insertPriceShocks(
	signal *Signal,
	symbols []string,
	prices map[string]float64,
	shocks []float64,
	eventAt time.Time,
) {
	for _, shock := range shocks {
		for _, symbol := range symbols {
			prices[symbol] *= shock
			insertTradeRow(signal, symbol, prices[symbol], 1, eventAt)
		}
	}
}

func seedTrades(signal *Signal, symbol string, base time.Time, count int, startPrice float64) {
	updates := make([]tradeUpdate, count)

	for index := range count {
		updates[index] = tradeUpdate{
			Symbol:    symbol,
			Price:     startPrice + float64(index)*0.01,
			Qty:       1,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		}
	}

	insertTrades(signal, symbol, updates)
}

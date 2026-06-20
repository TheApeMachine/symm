package causal

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func init() {
	viper.Set("signals.feed_ring_capacity", 64)
}

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

func feedTrade(
	signal *Signal,
	symbol, side string,
	price, qty float64,
	at time.Time,
) {
	raw, err := json.Marshal([]tradeUpdate{{
		Symbol:    symbol,
		Side:      side,
		Price:     price,
		Qty:       qty,
		Timestamp: at,
	}})

	if err != nil {
		panic(err)
	}

	insertTreeArtifact(signal, "trade", symbol, raw)
}

func TestHydrateNodeStoreFromTreeResetsFresh(testingTB *testing.T) {
	Convey("Given trades indexed in the tree", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		seedDefaultTrades(signal, "BTC/USD", baseTime)

		signal.hydrateNodeStoreFromTree()
		nodes := signal.nodeStore.Nodes("BTC/USD")
		firstLength := nodes.AlignedLength()

		signal.hydrateNodeStoreFromTree()
		secondLength := signal.nodeStore.Nodes("BTC/USD").AlignedLength()

		Convey("It should rebuild without duplicating ladder history", func() {
			So(firstLength, ShouldBeGreaterThanOrEqualTo, causalMinHistory)
			So(secondLength, ShouldEqual, firstLength)
		})
	})
}

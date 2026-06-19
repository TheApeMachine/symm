package fluid

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func insertTreeIngestFacts(signal *Signal, role, scope string, payload any) {
	raw, err := json.Marshal(payload)

	if err != nil {
		panic(err)
	}

	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload(raw)
	signal.tree.Insert(artifact.Prefix(), artifact.Marshal())
	artifact.Release()
}

func TestSignalMarketFacts(testingTB *testing.T) {
	Convey("Given ticker rows indexed in the tree", testingTB, func() {
		signal := NewSignal(context.Background(), qpool.NewQ[any](context.Background(), 1, 2, nil), dmt.NewTree(""))
		scope := "BTC/EUR"
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		insertTreeIngestFacts(signal, "ticker", scope, []TickerUpdate{{
			Symbol:    scope,
			Last:      100,
			Bid:       99.5,
			Ask:       100.5,
			Volume:    12,
			Timestamp: feedAt,
		}})

		signal.hydrateRegistryFromTree()
		facts := signal.MarketFacts(scope)

		Convey("It should expose quote context from hydrated registry state", func() {
			So(facts.Price, ShouldEqual, 100)
			So(facts.Volume, ShouldEqual, 12)
			So(facts.ObservedAt, ShouldEqual, feedAt)
		})
	})
}

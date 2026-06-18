package fluid

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
)

func feedArtifact(role, scope string, payload any) *datura.Artifact {
	raw, err := json.Marshal(payload)

	if err != nil {
		panic(err)
	}

	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload(raw)

	return artifact
}

func TestSignalMarketFacts(testingTB *testing.T) {
	Convey("Given a fluid signal without ingested relay rows", testingTB, func() {
		signal := NewSignal(context.Background(), qpool.NewQ[any](context.Background(), 1, 2, nil))
		scope := "BTC/EUR"

		ticker := feedArtifact("ticker", scope, []TickerUpdate{{
			Symbol: scope,
			Last:   100,
			Bid:    99.5,
			Ask:    100.5,
			Volume: 12,
		}})
		_ = signal.Update(ticker)
		ticker.Release()

		facts := signal.MarketFacts(scope)

		Convey("It should return empty facts while Update ingest is stubbed", func() {
			So(facts.Price, ShouldEqual, 0)
			So(facts.Volume, ShouldEqual, 0)
			So(facts.Spread, ShouldEqual, 0)
			So(facts.ObservedAt.IsZero(), ShouldBeFalse)
		})
	})
}

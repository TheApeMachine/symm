package signal

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
)

/*
MarketArtifact builds a routed feed artifact with a JSON array payload.
*/
func MarketArtifact(role string, payload any) *datura.Artifact {
	raw, err := sonic.Marshal(payload)

	if err != nil || len(raw) == 0 {
		return nil
	}

	return datura.Acquire(role, datura.Artifact_Type_json).
		WithRole(role).
		WithPayload(raw)
}

/*
TradeArtifact builds a trade feed artifact from records.
*/
func TradeArtifact(records ...TradeRecord) *datura.Artifact {
	return MarketArtifact("trade", records)
}

/*
BookArtifact builds a book feed artifact from records.
*/
func BookArtifact(records ...BookRecord) *datura.Artifact {
	return MarketArtifact("book", records)
}

/*
TickerArtifact builds a ticker feed artifact from records.
*/
func TickerArtifact(records ...TickerRecord) *datura.Artifact {
	return MarketArtifact("ticker", records)
}

/*
MeasurementArtifact builds a measurement scope trigger artifact.
*/
func MeasurementArtifact(scope string) *datura.Artifact {
	return datura.Acquire("measurement", datura.Artifact_Type_json).
		WithRole("measurement").
		WithScope(scope)
}

/*
VisitTickers calls visit for each ticker row with a positive last price.
*/
func VisitTickers(artifact *datura.Artifact, visit func(symbol string, last float64) bool) {
	if artifact == nil || visit == nil {
		return
	}

	datura.PayloadEach(artifact, func(index int, element ast.Node) bool {
		symbol, symbolOK := payloadString(element, "symbol")
		last, lastOK := payloadFloat(element, "last")

		if !symbolOK || symbol == "" || !lastOK || last <= 0 {
			return true
		}

		return visit(symbol, last)
	})
}

func RFC3339Time(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}

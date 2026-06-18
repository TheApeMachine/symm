package trader

import (
	"encoding/json"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	. "github.com/theapemachine/symm/signal"
)

func insertClassifierMeasurement(
	tree *dmt.Tree,
	origin, scope string,
	categoryIndex int,
	confidence float64,
) {
	artifact := datura.Acquire(origin, datura.Artifact_Type_json).
		WithRole("measurement").
		WithScope(scope)
	artifact.WithAttribute("classifier.category", categoryIndex)
	artifact.WithAttribute("classifier.confidence", confidence)
	artifact.WithAttribute("classifier.strength", 1.0)

	InsertMeasurement(tree, artifact)
}

func insertTickerQuote(tree *dmt.Tree, symbol string, last, bid, ask float64) {
	payload, _ := json.Marshal(map[string]any{
		"channel": "ticker",
		"type":    "update",
		"data": []map[string]any{{
			"symbol": symbol,
			"last":   last,
			"bid":    bid,
			"ask":    ask,
		}},
	})

	artifact := datura.Acquire("test", datura.Artifact_Type_json).
		WithRole("ticker").
		WithScope(symbol).
		WithPayload(payload)

	InsertTreeArtifact(tree, artifact)
}

func storyBalanceArtifact(balances logic.Balances) *datura.Artifact {
	payload, _ := json.Marshal(balances)

	return datura.Acquire("test", datura.Artifact_Type_json).
		WithRole("balances").
		WithPayload(payload)
}

func isExitOrderArtifact(artifact *datura.Artifact) bool {
	if artifact == nil {
		return false
	}

	payload, err := artifact.DecryptPayload()

	if err != nil || len(payload) == 0 {
		return false
	}

	var envelope struct {
		Params json.RawMessage `json:"params"`
	}

	if json.Unmarshal(payload, &envelope) != nil {
		return false
	}

	var params struct {
		OrderType string `json:"order_type"`
		Side      string `json:"side"`
	}

	if json.Unmarshal(envelope.Params, &params) != nil {
		return false
	}

	if params.OrderType == "settle_position" {
		return true
	}

	return params.Side == "sell"
}

func collectPrivateOrders(
	received <-chan *datura.Artifact,
	want int,
) []*datura.Artifact {
	captured := make([]*datura.Artifact, 0, want)
	deadline := time.After(2 * time.Second)

	for len(captured) < want {
		select {
		case artifact := <-received:
			if datura.Peek[string](artifact, "role") != "orders" {
				continue
			}

			captured = append(captured, artifact)
		case <-deadline:
			return captured
		}
	}

	return captured
}

func drainPrivateOrders(received <-chan *datura.Artifact) {
	for {
		select {
		case <-received:
		default:
			return
		}
	}
}

package causal

import (
	"encoding/json"
	"time"

	"github.com/theapemachine/datura"
)

func artifactPayload(artifact *datura.Artifact) ([]byte, bool) {
	if artifact == nil || !artifact.HasEncryptedPayload() {
		return nil, false
	}

	payload := artifact.DecryptPayload()

	if len(payload) == 0 {
		return nil, false
	}

	return payload, true
}

type tradeUpdate struct {
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
	Side      string    `json:"side"`
	Timestamp time.Time `json:"timestamp"`
}

func (signal *Signal) hydrateNodeStoreFromTree() {
	if signal == nil || signal.tree == nil {
		return
	}

	signal.nodeStore = NewNodeStore()

	for inbound := range signal.tree.Seek([]byte("trade/")) {
		signal.observeTradeArtifact(inbound)
	}
}

func (signal *Signal) observeTradeArtifact(artifact *datura.Artifact) {
	payload, payloadOK := artifactPayload(artifact)

	if !payloadOK {
		return
	}

	var update tradeUpdate

	if json.Unmarshal(payload, &update) == nil && update.Symbol != "" {
		signal.observeTradeUpdate(update)
		return
	}

	var updates []tradeUpdate

	if json.Unmarshal(payload, &updates) != nil {
		return
	}

	for _, row := range updates {
		signal.observeTradeUpdate(row)
	}
}

func (signal *Signal) observeTradeUpdate(update tradeUpdate) {
	if update.Symbol == "" || update.Price <= 0 || update.Qty <= 0 {
		return
	}

	raw, err := json.Marshal(update)

	if err != nil {
		return
	}

	signal.nodeStore.Observe(update.Symbol, raw)
}

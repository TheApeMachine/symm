package public

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/kraken/types"
)

// TestRecordPairsPersistsAndSubscribes verifies the instrument frame is the
// symbol source of truth: online pairs are recorded to the tree per symbol with
// their full metadata, a snapshot (re)subscribes every pair, and an update only
// surfaces genuinely new ones.
func TestRecordPairsPersistsAndSubscribes(t *testing.T) {
	ws := &WebSocket{tree: dmt.NewTree("")}

	snapshot := []byte(`{"pairs":[
		{"symbol":"ETH/USD","status":"online","tick_size":0.01,"qty_min":0.001},
		{"symbol":"SOL/USD","status":"online","tick_size":0.001,"qty_min":0.01},
		{"symbol":"DEAD/USD","status":"delisted"}
	]}`)

	fresh, err := ws.recordPairs(snapshot, "snapshot")
	if err != nil {
		t.Fatalf("recordPairs returned error: %v", err)
	}

	if len(fresh) != 2 {
		t.Fatalf("want 2 online pairs from snapshot, got %d: %v", len(fresh), fresh)
	}

	// The full per-pair metadata must be queryable from the tree by symbol.
	raw, ok := ws.tree.Get(instrumentKey("ETH/USD"))
	if !ok {
		t.Fatal("ETH/USD instrument metadata not stored in tree")
	}

	artifact := datura.Acquire("test", datura.APPJSON)
	if _, err := artifact.Unpack(raw); err != nil {
		t.Fatalf("failed to read stored instrument artifact: %v", err)
	}

	var meta struct {
		TickSize float64 `json:"tick_size"`
		QtyMin   float64 `json:"qty_min"`
	}
	if err := sonic.Unmarshal(artifact.DecryptPayload(), &meta); err != nil {
		t.Fatalf("failed to parse stored metadata: %v", err)
	}
	if meta.TickSize != 0.01 || meta.QtyMin != 0.001 {
		t.Fatalf("metadata not preserved: tick_size=%v qty_min=%v", meta.TickSize, meta.QtyMin)
	}

	// An update re-reporting ETH/USD plus a new AVAX/USD should surface only the
	// newcomer for subscription (ETH is already in the tree).
	update := []byte(`{"pairs":[
		{"symbol":"ETH/USD","status":"online","tick_size":0.01},
		{"symbol":"AVAX/USD","status":"online","tick_size":0.001}
	]}`)

	again, err := ws.recordPairs(update, "update")
	if err != nil {
		t.Fatalf("recordPairs returned error: %v", err)
	}
	if len(again) != 1 || again[0] != "AVAX/USD" {
		t.Fatalf("want only fresh AVAX/USD from update, got %v", again)
	}

	// A fresh snapshot (as on reconnect) re-subscribes every online pair even
	// though they are already in the tree.
	reconnect, err := ws.recordPairs(snapshot, "snapshot")
	if err != nil {
		t.Fatalf("recordPairs returned error: %v", err)
	}
	if len(reconnect) != 2 {
		t.Fatalf("snapshot should re-subscribe all online pairs, got %v", reconnect)
	}
}

func TestRecordPairsFiltersConfiguredQuoteCurrency(t *testing.T) {
	oldQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() {
		viper.Set("market.quote_currency", oldQuote)
	})

	ws := &WebSocket{tree: dmt.NewTree("")}

	snapshot := []byte(`{"pairs":[
		{"symbol":"ETH/USD","status":"online","tick_size":0.1},
		{"symbol":"ETH/EUR","status":"online","tick_size":0.01}
	]}`)

	fresh, err := ws.recordPairs(snapshot, "snapshot")
	if err != nil {
		t.Fatalf("recordPairs returned error: %v", err)
	}

	if len(fresh) != 1 || fresh[0] != "ETH/USD" {
		t.Fatalf("want only USD pair subscribed, got %v", fresh)
	}

	if _, ok := ws.tree.Get(instrumentKey("ETH/EUR")); ok {
		t.Fatal("cross-quote instrument metadata should not be stored")
	}
}

func TestRecordPairsFiltersNonOpportunityBases(t *testing.T) {
	oldQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() {
		viper.Set("market.quote_currency", oldQuote)
	})

	ws := &WebSocket{tree: dmt.NewTree("")}

	snapshot := []byte(`{"pairs":[
		{"symbol":"BTC/USD","status":"online","tick_size":0.1},
		{"symbol":"EUR/USD","status":"online","tick_size":0.00001},
		{"symbol":"GBP/USD","status":"online","tick_size":0.00001},
		{"symbol":"USDT/USD","status":"online","tick_size":0.0001},
		{"symbol":"DOGE/USD","status":"online","tick_size":0.000001}
	]}`)

	fresh, err := ws.recordPairs(snapshot, "snapshot")
	if err != nil {
		t.Fatalf("recordPairs returned error: %v", err)
	}

	if len(fresh) != 1 || fresh[0] != "DOGE/USD" {
		t.Fatalf("want only chaseable DOGE/USD subscribed, got %v", fresh)
	}

	for _, symbol := range []string{"BTC/USD", "EUR/USD", "GBP/USD", "USDT/USD"} {
		if _, ok := ws.tree.Get(instrumentKey(symbol)); ok {
			t.Fatalf("%s should not be stored as a tradable instrument", symbol)
		}
	}
}

func TestLoadAssetPairsPersistsFeeSchedulesByTradingSymbol(t *testing.T) {
	oldQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() {
		viper.Set("market.quote_currency", oldQuote)
	})

	rest := &Rest{tree: dmt.NewTree("")}
	count, err := rest.ingestAssetPairs([]byte(`{
		"error": [],
		"result": {
			"XDGUSD": {
				"wsname": "XDG/USD",
				"status": "online",
				"fees": [[0, 0.4]],
				"fees_maker": [[0, 0.25]],
				"fee_volume_currency": "ZUSD"
			}
		}
	}`))

	if err != nil {
		t.Fatalf("ingestAssetPairs returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("want XDG/USD plus DOGE/USD aliases, got %d schedules", count)
	}

	for _, scope := range []string{"XDG/USD", "DOGE/USD"} {
		artifact := artifactWithScope(rest.tree, "assetpairs", scope)
		if artifact == nil {
			t.Fatalf("%s AssetPairs schedule was not stored", scope)
		}
		defer artifact.Release()

		if taker := datura.Peek[float64](artifact, "fees", 0, 1); taker != 0.4 {
			t.Fatalf("%s taker fee=%v, want 0.4", scope, taker)
		}
		if maker := datura.Peek[float64](artifact, "fees_maker", 0, 1); maker != 0.25 {
			t.Fatalf("%s maker fee=%v, want 0.25", scope, maker)
		}
	}
}

func TestPersistMarketFrameFiltersConfiguredQuoteCurrency(t *testing.T) {
	oldQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() {
		viper.Set("market.quote_currency", oldQuote)
	})

	ws := &WebSocket{tree: dmt.NewTree("")}

	if err := ws.persistMarketFrame(types.SocketMessage{
		Channel: "ticker",
		Type:    "update",
		Data: []byte(`[
			{"symbol":"ETH/USD","bid":99,"ask":101,"last":100,"volume":10},
			{"symbol":"ETH/EUR","bid":199,"ask":201,"last":200,"volume":20}
		]`),
	}); err != nil {
		t.Fatalf("persist market frame failed: %v", err)
	}

	if firstArtifact(ws.tree.Seek([]byte("ticker/ETH/USD/"))) == nil {
		t.Fatal("ETH/USD ticker was not indexed")
	}

	if firstArtifact(ws.tree.Seek([]byte("ticker/ETH/EUR/"))) != nil {
		t.Fatal("ETH/EUR ticker should not be indexed when quote_currency=USD")
	}
}

func TestPersistMarketFrameScopesRowsBySymbol(t *testing.T) {
	ws := &WebSocket{tree: dmt.NewTree("")}

	if err := ws.persistMarketFrame(types.SocketMessage{
		Channel: "ticker",
		Type:    "update",
		Data: []byte(`[
			{"symbol":"ETH/USD","bid":99,"ask":101,"last":100,"volume":10},
			{"symbol":"DOGE/USD","bid":0.199,"ask":0.201,"last":0.2,"volume":20}
		]`),
	}); err != nil {
		t.Fatalf("persist market frame failed: %v", err)
	}

	eth := firstArtifact(ws.tree.Seek([]byte("ticker/ETH/USD/")))
	if eth == nil {
		t.Fatal("ETH/USD ticker was not indexed by role/symbol")
	}

	scope, _ := eth.Scope()
	if scope != "ETH/USD" {
		t.Fatalf("scope=%q, want ETH/USD", scope)
	}

	var frame struct {
		Channel string `json:"channel"`
		Type    string `json:"type"`
		Data    []struct {
			Symbol string  `json:"symbol"`
			Last   float64 `json:"last"`
		} `json:"data"`
	}
	if err := sonic.Unmarshal(eth.DecryptPayload(), &frame); err != nil {
		t.Fatalf("failed to decode scoped payload: %v", err)
	}
	if frame.Channel != "ticker" || frame.Type != "update" {
		t.Fatalf("frame metadata not preserved: channel=%q type=%q", frame.Channel, frame.Type)
	}
	if len(frame.Data) != 1 || frame.Data[0].Symbol != "ETH/USD" || frame.Data[0].Last != 100 {
		t.Fatalf("ETH payload not scoped to one row: %+v", frame.Data)
	}

	if firstArtifact(ws.tree.Seek([]byte("ticker/DOGE/USD/"))) == nil {
		t.Fatal("DOGE/USD ticker was not indexed by role/symbol")
	}
}

func firstArtifact(seq func(func(*datura.Artifact) bool)) *datura.Artifact {
	var first *datura.Artifact
	seq(func(candidate *datura.Artifact) bool {
		first = candidate
		return false
	})
	return first
}

func artifactWithScope(tree *dmt.Tree, role, scope string) *datura.Artifact {
	for candidate := range tree.Seek([]byte(role + "/")) {
		candidateScope, _ := candidate.Scope()
		if candidateScope == scope {
			return candidate
		}
		candidate.Release()
	}

	return nil
}

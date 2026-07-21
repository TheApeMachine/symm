package tradevolume

import (
	"embed"
	"iter"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

//go:embed fixtures/*.json
var fixtureFiles embed.FS

/*
Fixture yields one Kraken TradeVolume REST response whose fee map covers the
complete simulated symbol universe.
*/
type Fixture struct {
	payload []byte
}

/*
NewMarket injects every simulated pair into the TradeVolume response template.
*/
func NewMarket(symbols []string) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/response.json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "trade volume fixture load failed", err))
	}

	var payload map[string]any

	if err := sonic.Unmarshal(raw, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "trade volume fixture decode failed", err))
	}

	result := payload["result"].(map[string]any)
	template := result["fees"].(map[string]any)["PAIR"]
	fees := make(map[string]any, len(symbols))

	for _, symbol := range symbols {
		fees[strings.ReplaceAll(symbol, "/", "")] = template
	}

	result["fees"] = fees
	encoded, err := sonic.Marshal(payload)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "trade volume fixture encode failed", err))
	}

	return &Fixture{payload: encoded}
}

/*
Generate yields the ready Kraken REST response.
*/
func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		yield(fixture.payload)
	}
}

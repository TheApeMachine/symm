package tradevolume

import (
	"embed"
	"iter"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	testtypes "github.com/theapemachine/symm/tests/types"
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
NewProfiles builds per-symbol taker and maker fee maps from the same profiles
used by AssetPairs and simulated execution accounting.
*/
func NewProfiles(symbols []*testtypes.Symbol) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/response.json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "trade volume fixture load failed", err))
	}

	var payload map[string]any

	if err := sonic.Unmarshal(raw, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "trade volume fixture decode failed", err))
	}

	result := payload["result"].(map[string]any)
	fees := make(map[string]any, len(symbols))
	makers := make(map[string]any, len(symbols))

	for _, symbol := range symbols {
		pair := strings.ReplaceAll(symbol.Pair, "/", "")
		taker := strconv.FormatFloat(symbol.TakerFeePercent, 'f', -1, 64)
		maker := strconv.FormatFloat(symbol.MakerFeePercent, 'f', -1, 64)
		fees[pair] = map[string]any{
			"fee": taker, "minfee": taker, "maxfee": taker,
			"tiervolume": "0",
		}
		makers[pair] = map[string]any{
			"fee": maker, "minfee": maker, "maxfee": maker,
			"tiervolume": "0",
		}
	}

	result["fees"] = fees
	result["fees_maker"] = makers
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

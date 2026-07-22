package balances

import (
	"embed"
	"encoding/json"
	"fmt"
	"iter"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

//go:embed fixtures/*.json
var fixtureFiles embed.FS

/*
FixtureType selects the Kraken balance envelope represented by a fixture.
*/
type FixtureType string

const (
	SNAPSHOT FixtureType = "snapshot"
	UPDATE   FixtureType = "update"
)

/*
Fixture represents a Balances update as it is coming through the WebSocket
from the Kraken exchange. You can either get a single balance snapshot, or
generate a sequence of balance updates, which is useful in different kinds
of testing scenarios.
*/
type Fixture struct {
	horizon  int
	sequence [][]byte
}

/*
Frame injects the current paper ledger into the Kraken balance template.
*/
func Frame(
	balances map[string]float64,
	reserved map[string]float64,
	typ FixtureType,
	sequence int64,
) []byte {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(SNAPSHOT) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "balances fixture load failed", err))
	}

	var payload map[string]any

	if err := sonic.Unmarshal(raw, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "balances fixture decode failed", err))
	}

	assets := make([]string, 0, len(balances))

	for asset := range balances {
		assets = append(assets, asset)
	}

	sort.Strings(assets)
	rows := make([]map[string]any, len(assets))

	for index, asset := range assets {
		balance := balances[asset]
		locked := reserved[asset]
		rows[index] = map[string]any{
			"asset":       asset,
			"asset_class": "currency",
			"balance":     balance,
			"available":   balance - locked,
			"reserved":    locked,
			"wallets": []map[string]any{{
				"type":    "spot",
				"id":      "main",
				"balance": balance,
			}},
		}
	}

	payload["data"] = rows
	payload["type"] = string(typ)
	payload["sequence"] = sequence
	encoded, err := sonic.Marshal(payload)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "balances fixture encode failed", err))
	}

	return encoded
}

/*
NewFixture loads a snapshot or a deterministic sequence of balance updates.
*/
func NewFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "balances fixture load failed", err))
	}

	fixture := &Fixture{horizon: horizon}

	if typ == SNAPSHOT {
		fixture.sequence = [][]byte{raw}
		return fixture
	}

	if typ != UPDATE || horizon < 1 {
		panic(errnie.Err(
			errnie.Validation,
			"balances fixture update horizon must be positive",
			nil,
		))
	}

	return fixture.sequencer(raw)
}

/*
NewMarket filters the Kraken balance snapshot template to the simulated quote
wallet so the real paper boot starts from explicit fixture inventory.
*/
func NewMarket(quote string) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(SNAPSHOT) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "balances fixture load failed", err))
	}

	var payload map[string]any

	if err := sonic.Unmarshal(raw, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "balances fixture decode failed", err))
	}

	for _, entry := range payload["data"].([]any) {
		row := entry.(map[string]any)

		if row["asset"] != quote {
			continue
		}

		row["available"] = row["balance"]
		row["reserved"] = 0.0

		payload["data"] = []any{row}
		encoded, encodeErr := sonic.Marshal(payload)

		if encodeErr != nil {
			panic(errnie.Err(errnie.Validation, "balances fixture encode failed", encodeErr))
		}

		return &Fixture{sequence: [][]byte{encoded}}
	}

	panic(errnie.Err(errnie.NotFound, "balances fixture quote wallet missing", nil))
}

/*
generate a sequence of balances updates.
*/
func (fixture *Fixture) sequencer(raw []byte) *Fixture {
	var base map[string]any

	if err := sonic.Unmarshal(raw, &base); err != nil {
		panic(errnie.Err(errnie.Validation, "balances fixture decode failed", err))
	}

	fixture.sequence = make([][]byte, fixture.horizon)

	rawData, ok := base["data"].([]any)

	if !ok || len(rawData) == 0 {
		panic(errnie.Err(errnie.Validation, "balances fixture data missing", nil))
	}

	firstItem, ok := rawData[0].(map[string]any)

	if !ok {
		panic(errnie.Err(errnie.Validation, "balances fixture row missing", nil))
	}

	baseSequence, sequenceExists := base["sequence"].(float64)
	currentBalance, balanceExists := firstItem["balance"].(float64)
	timestamp, timestampExists := firstItem["timestamp"].(string)

	if !sequenceExists || !balanceExists || !timestampExists {
		panic(errnie.Err(
			errnie.Validation,
			"balances fixture sequence, balance, and timestamp required",
			nil,
		))
	}

	currentTime, err := time.Parse(time.RFC3339Nano, timestamp)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "balances fixture timestamp invalid", err))
	}

	asset, assetExists := firstItem["asset"].(string)
	assetClass, assetClassExists := firstItem["asset_class"].(string)
	category, categoryExists := firstItem["category"].(string)
	walletType, walletTypeExists := firstItem["wallet_type"].(string)
	walletID, walletIDExists := firstItem["wallet_id"].(string)

	if !assetExists || !assetClassExists || !categoryExists ||
		!walletTypeExists || !walletIDExists {
		panic(errnie.Err(errnie.Validation, "balances fixture identity fields required", nil))
	}

	rng := rand.New(rand.NewSource(42))

	for index := range fixture.horizon {
		durationSec := 10 + rng.Intn(110)
		currentTime = currentTime.Add(time.Duration(durationSec) * time.Second)
		direction := 1.0

		if rng.Float64() >= 0.5 {
			direction = -1
		}

		amount := direction * rng.Float64() * 0.05

		if direction < 0 && currentBalance+amount < 0 {
			amount = -currentBalance * 0.5
		}

		fee := math.Abs(amount) * 0.0026
		currentBalance += amount

		amount = math.Round(amount*1e8) / 1e8
		fee = math.Round(fee*1e8) / 1e8
		currentBalance = math.Round(currentBalance*1e8) / 1e8

		stepData := map[string]any{
			"ledger_id":   fmt.Sprintf("LID-%04d-%04d", index, rng.Intn(10000)),
			"ref_id":      fmt.Sprintf("REF-%04d-%04d", index, rng.Intn(10000)),
			"timestamp":   currentTime.Format(time.RFC3339Nano),
			"type":        "trade",
			"asset":       asset,
			"asset_class": assetClass,
			"category":    category,
			"wallet_type": walletType,
			"wallet_id":   walletID,
			"amount":      amount,
			"fee":         fee,
			"balance":     currentBalance,
		}

		step := map[string]any{
			"channel":  base["channel"],
			"type":     base["type"],
			"data":     []any{stepData},
			"sequence": int64(baseSequence) + int64(index),
		}

		marshaled, err := json.Marshal(step)

		if err != nil {
			panic(errnie.Err(errnie.Validation, "balances fixture encode failed", err))
		}

		fixture.sequence[index] = marshaled
	}

	return fixture
}

/*
Generate yields fixtures.
*/
func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for _, seq := range fixture.sequence {
			if !yield(seq) {
				return
			}
		}
	}
}

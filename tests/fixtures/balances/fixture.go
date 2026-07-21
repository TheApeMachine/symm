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
func Frame(balances map[string]float64, typ FixtureType) []byte {
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
		rows[index] = map[string]any{
			"asset":       asset,
			"asset_class": "currency",
			"balance":     balance,
			"available":   balance,
			"reserved":    0,
			"wallets": []map[string]any{{
				"type":    "spot",
				"id":      "main",
				"balance": balance,
			}},
		}
	}

	payload["data"] = rows
	payload["type"] = string(typ)
	encoded, err := sonic.Marshal(payload)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "balances fixture encode failed", err))
	}

	return encoded
}

func NewFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")
	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	fixture := &Fixture{
		horizon: horizon,
	}

	if typ == SNAPSHOT {
		fixture.sequence = make([][]byte, 1)
		fixture.sequence[0] = raw
		return fixture
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
		errnie.Error(err)
		return fixture
	}

	steps := fixture.horizon
	fixture.sequence = make([][]byte, steps)

	// Safely assert the 'data' array and the first item
	rawData, ok := base["data"].([]any)
	if !ok || len(rawData) == 0 {
		return fixture
	}

	firstItem, ok := rawData[0].(map[string]any)
	if !ok {
		return fixture
	}

	// Extract starting state with type assertions
	baseSeq, _ := base["sequence"].(float64)
	currentBalance, _ := firstItem["balance"].(float64)
	timestampStr, _ := firstItem["timestamp"].(string)

	currentTime, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		// Fallback to current time if parsing fails
		currentTime = time.Now()
	}

	// Capture static fields to populate downstream updates
	asset, _ := firstItem["asset"].(string)
	assetClass, _ := firstItem["asset_class"].(string)
	category, _ := firstItem["category"].(string)
	walletType, _ := firstItem["wallet_type"].(string)
	walletID, _ := firstItem["wallet_id"].(string)

	rng := rand.New(rand.NewSource(42))

	for i := 0; i < steps; i++ {
		// Move time forward by a pseudo-random interval (between 10s and 2m)
		durationSec := 10 + rng.Intn(110)
		currentTime = currentTime.Add(time.Duration(durationSec) * time.Second)

		// Generate random amounts without allowing a negative balance
		var amount float64

		if rng.Float64() < 0.5 {
			// Buy (up to 0.05 BTC)
			amount = rng.Float64() * 0.05
		} else {
			// Sell (up to 0.05 BTC)
			amount = -rng.Float64() * 0.05
			if currentBalance+amount < 0 {
				amount = -currentBalance * 0.5
			}
		}

		fee := math.Abs(amount) * 0.0026
		currentBalance += amount

		// Round to 8 decimal places for crypto precision
		amount = math.Round(amount*1e8) / 1e8
		fee = math.Round(fee*1e8) / 1e8
		currentBalance = math.Round(currentBalance*1e8) / 1e8

		// Rebuild the data entry
		stepData := map[string]any{
			"ledger_id":   fmt.Sprintf("LID-%04d-%04d", i, rng.Intn(10000)),
			"ref_id":      fmt.Sprintf("REF-%04d-%04d", i, rng.Intn(10000)),
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

		// Rebuild the outer envelope to ensure sequence changes on every iteration
		step := map[string]any{
			"channel":  base["channel"],
			"type":     base["type"],
			"data":     []any{stepData},
			"sequence": int64(baseSeq) + int64(i),
		}

		marshaled, err := json.Marshal(step)
		if err != nil {
			panic(err)
		}
		fixture.sequence[i] = marshaled
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

//go:build !race

package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/backtest/hindsight"
	"github.com/theapemachine/symm/cmd"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

/*
TestMeasuredSqliteReplayProfitability replays one recorded SQLite capture
through the same in-process production stack the playback driver uses, and
reports the realized economics next to the tape's perfect-execution ceiling.
It is gated behind SYMM_MEASURE because it boots the full Metal + broker stack
against a large capture store.

This is the profitability proof surface: NetPnL is the system's real outcome
over identical market bytes, and the hindsight ceiling is the tape property
the system was competing against.
*/
func TestMeasuredSqliteReplayProfitability(t *testing.T) {
	if os.Getenv("SYMM_MEASURE") == "" {
		t.Skip("SYMM_MEASURE not set; skipping sqlite replay measurement")
	}

	dataPath := "/Users/theapemachine/.symm/data"

	sourceStore, err := backtest.NewStore(filepath.Join(dataPath, "symm.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer sourceStore.Close()

	captures, err := sourceStore.ListCaptures()
	if err != nil {
		t.Fatal(err)
	}

	if len(captures) == 0 {
		t.Skip("no captures recorded")
	}

	captureID := captures[0].ID

	if os.Getenv("SYMM_MEASURE_CAPTURE") != "" {
		if value, parseErr := json.Number(os.Getenv("SYMM_MEASURE_CAPTURE")).Int64(); parseErr == nil && value > 0 {
			captureID = value
		}
	}
	startedAt, endedAt, err := sourceStore.Bounds(captureID)
	if err != nil {
		t.Fatal(err)
	}

	symbols, err := captureSymbolsFromStore(sourceStore, captureID)
	if err != nil || len(symbols) == 0 {
		t.Fatalf("capture has no symbols: %v", err)
	}

	config := testtypes.NewScenarioConfig(symbols)
	config.StartTime = startedAt
	config.InitialBalances = map[string]float64{"USD": 200}
	config.Execution.DepthLevels = 10

	previousModel := viper.Get("trading.model")
	viper.Set("trading.model", "real")
	previousPace := viper.Get("market.subscribe.pace")
	viper.Set("market.subscribe.pace", time.Millisecond)
	previousDataPath := viper.GetString("system.data_path")
	tmpDataPath := t.TempDir()
	viper.Set("system.data_path", tmpDataPath)
	t.Cleanup(func() {
		viper.Set("trading.model", previousModel)
		viper.Set("market.subscribe.pace", previousPace)
		viper.Set("system.data_path", previousDataPath)
	})

	previousKey, previousSecret := os.Getenv("KRAKEN_API_KEY"), os.Getenv("KRAKEN_API_SECRET")
	os.Setenv("KRAKEN_API_KEY", "fixture-key")
	os.Setenv("KRAKEN_API_SECRET", "Zml4dHVyZS1zZWNyZXQ=")
	t.Cleanup(func() {
		os.Setenv("KRAKEN_API_KEY", previousKey)
		os.Setenv("KRAKEN_API_SECRET", previousSecret)
	})

	market, err := NewMarketWithScenario(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer market.Close()

	market.WithAutoFill(config.Execution)

	uiChannel := make(chan []byte, 1024)
	publicFeed, privateFeed := market.Feeds()
	thesis := types.NewThesis(context.Background(), uiChannel)
	system := cmd.BootWithHub(
		context.Background(), thesis, publicFeed, privateFeed, uiChannel, nil,
	)

	if system == nil {
		t.Fatal("boot produced no system")
	}
	defer system.Close()

	market.Drive(system)

	frames, release, err := sourceStore.Frames(captureID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	replayed := 0
	frameLimit := 0

	if os.Getenv("SYMM_MEASURE_LIMIT") != "" {
		if value, parseErr := json.Number(os.Getenv("SYMM_MEASURE_LIMIT")).Int64(); parseErr == nil && value > 0 {
			frameLimit = int(value)
		}
	}

	for {
		frame, ok := frames()
		if !ok {
			break
		}

		if os.Getenv("SYMM_MEASURE_TRACE") != "" && replayed < 20 && frame.Endpoint != "level3" {
			var header struct {
				Channel string `json:"channel"`
			}
			_ = json.Unmarshal(frame.Payload, &header)

			delivered := false

			switch frame.Endpoint {
			case "public":
				delivered = market.Public.Publish(header.Channel, frame.Payload)
			case "private":
				delivered = market.Private.Publish(header.Channel, frame.Payload)
			}

			t.Logf("TRACE frame %d %s/%s delivered=%v tick=%d", replayed, frame.Endpoint, header.Channel, delivered, system.Thesis.Tick)
		} else {
			publishCapturedFrame(market, frame)
		}

		if err := system.Sync(context.Background(), frame.ReceivedAt); err != nil {
			t.Fatalf("sync stack at %s: %v", frame.ReceivedAt, err)
		}

		replayed++

		if frameLimit > 0 && replayed >= frameLimit {
			break
		}
	}

	report := market.Report()
	tape := sqliteHindsightCeiling(t, sourceStore, captureID, startedAt, tmpDataPath)
	realizedPct := report.Economics.NetPnL / 200 * 100

	t.Logf(
		"MEASURE capture %d (%s→%s): frames=%d net=%.6f realizedPct=%.4f%% gross=%.6f fees=%.6f filled=%d ceiling=%.6f%% sliceRealized=%.6f%% missed=%.6f%% legs=%d decisions=%d",
		captureID, startedAt.Format(time.RFC3339), endedAt.Format(time.RFC3339),
		replayed,
		report.Economics.NetPnL, realizedPct, report.Economics.GrossPnL, report.Economics.Fees,
		report.Mechanics.Filled,
		tape.Total.UpboundPct, tape.Total.RealizedPct, tape.Total.MissedPct, tape.Total.Legs,
		len(tape.Decisions),
	)

	if os.Getenv("SYMM_MEASURE_OPPORTUNITIES") != "" {
		for _, symbol := range tape.Symbols {
			if symbol.MissedLegs == 0 {
				continue
			}

			for _, opportunity := range symbol.Opportunities {
				if !opportunity.Missed {
					continue
				}

				t.Logf(
					"MISSED %s buy=%s@%.6f sell=%s@%.6f profit=%.4f%% signalAt=%s thesis=%.4f graph=%.4f opp=%v type=%s",
					symbol.Symbol,
					opportunity.Leg.BuyAt.Format("15:04:05"),
					opportunity.Leg.BuyPrice,
					opportunity.Leg.SellAt.Format("15:04:05"),
					opportunity.Leg.SellPrice,
					opportunity.Leg.ProfitPct*100,
					opportunity.Signal.At.Format("15:04:05"),
					opportunity.Signal.ThesisScore,
					opportunity.Signal.GraphScore,
					opportunity.Signal.Opportunity,
					opportunity.Signal.Type,
				)
			}
		}
	}
}

func publishCapturedFrame(market *Market, frame backtest.Frame) {
	var header struct {
		Channel string `json:"channel"`
	}
	_ = json.Unmarshal(frame.Payload, &header)

	switch frame.Endpoint {
	case "level3":
		if err := market.Level3.WaitReady(market.ctx); err != nil {
			return
		}

		market.Level3.Publish(header.Channel, frame.Payload)
	case "public":
		if err := market.Public.WaitReady(market.ctx); err != nil {
			return
		}

		market.Public.Publish(header.Channel, frame.Payload)
	case "private":
		if err := market.Private.WaitReady(market.ctx); err != nil {
			return
		}

		market.Private.Publish(header.Channel, frame.Payload)
	}
}

func captureSymbolsFromStore(
	store *backtest.Store,
	captureID int64,
) ([]*testtypes.Symbol, error) {
	frames, release, err := store.Frames(captureID, time.Time{})
	if err != nil {
		return nil, err
	}
	defer release()

	prices := map[string]float64{}
	scanned := 0

	for {
		if scanned >= 20000 {
			break
		}

		frame, ok := frames()
		if !ok {
			break
		}

		scanned++

		if frame.Endpoint != "public" {
			continue
		}

		var header struct {
			Channel string `json:"channel"`
		}
		if json.Unmarshal(frame.Payload, &header) != nil || header.Channel != "ticker" {
			continue
		}

		var ticker struct {
			Data []struct {
				Symbol string  `json:"symbol"`
				Last   float64 `json:"last"`
			} `json:"data"`
		}
		if json.Unmarshal(frame.Payload, &ticker) != nil {
			continue
		}

		for _, row := range ticker.Data {
			if row.Symbol != "" && row.Last > 0 {
				prices[row.Symbol] = row.Last
			}
		}

		if len(prices) >= 512 {
			break
		}
	}

	symbols := make([]*testtypes.Symbol, 0, len(prices))
	index := int64(1)

	for pair, price := range prices {
		symbols = append(symbols, testtypes.NewSymbol(pair, price, index))
		index++
	}

	return symbols, nil
}

/*
measuredTape is the combined replay result: the aggregated economics and the
full per-symbol hindsight analysis including every missed leg's signal context.
*/
type measuredTape struct {
	Total     hindsight.PerSymbol
	Symbols   []hindsight.PerSymbol
	Decisions []hindsight.Decision
}

/*
sqliteHindsightCeiling reduces one capture's trade prints into their per-symbol
perfect-execution ceiling and, when a replayed decision store is provided, ties
each leg to the decision the system actually made — the reverse-engineering
surface for why a profitable move was missed.
*/
func sqliteHindsightCeiling(
	t *testing.T,
	sourceStore *backtest.Store,
	captureID int64,
	from time.Time,
	tmpDataPath string,
) measuredTape {
	t.Helper()

	reducer := hindsight.NewReducer()
	frames, release, err := sourceStore.Frames(captureID, from)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	for {
		frame, ok := frames()
		if !ok {
			break
		}

		if frame.Endpoint != "public" {
			continue
		}

		var header struct {
			Channel string `json:"channel"`
			Type    string `json:"type"`
		}
		if json.Unmarshal(frame.Payload, &header) != nil || header.Channel != "trade" {
			continue
		}

		if err := reducer.Ingest(frame.Payload, frame.ReceivedAt); err != nil {
			t.Fatalf("reduce trade frame: %v", err)
		}
	}

	decisions, err := replayDecisions(tmpDataPath, from)
	if err != nil {
		t.Fatal(err)
	}

	reports, err := hindsight.Analyze(reducer, decisions)
	if err != nil {
		t.Fatal(err)
	}

	var total hindsight.PerSymbol
	total.Symbol = "capture"

	for _, report := range reports {
		total.UpboundPct += report.UpboundPct
		total.RealizedPct += report.RealizedPct
		total.MissedPct += report.MissedPct
		total.Legs += report.Legs
		total.MissedLegs += report.MissedLegs
	}

	return measuredTape{
		Total:     total,
		Symbols:   reports,
		Decisions: decisions,
	}
}

/*
replayDecisions reads the decision moments the replay itself recorded into its
fresh store, gated to decisions at or after the capture start so a leg can only
be attributed to the decision stream of this replay.
*/
func replayDecisions(dataPath string, from time.Time) ([]hindsight.Decision, error) {
	store, err := backtest.NewStore(filepath.Join(dataPath, "symm.sqlite"))
	if err != nil {
		return nil, err
	}
	defer store.Close()

	next, err := store.Events()
	if err != nil {
		return nil, err
	}

	decisions := []hindsight.Decision{}

	for {
		kind, payload, at, ok := next()
		if !ok {
			break
		}

		if kind != "[]types.Decision" || at.Before(from) {
			continue
		}

		var batch []hindsight.Decision

		if decodeErr := json.Unmarshal(payload, &batch); decodeErr != nil {
			return nil, decodeErr
		}

		decisions = append(decisions, batch...)
	}

	return decisions, nil
}

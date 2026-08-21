//go:build !race

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/backtest/hindsight"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/nomagique/transport"
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

	viper.SetConfigFile(filepath.Join("..", "cmd", "cfg", "config.yml"))

	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}

	dataPath := "/Users/theapemachine/.symm/data"

	sourceStore, err := backtest.NewStore(filepath.Join(dataPath, "symm.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer sourceStore.Close()

	captureID := int64(0)

	if os.Getenv("SYMM_MEASURE_CAPTURE") != "" {
		if value, parseErr := json.Number(os.Getenv("SYMM_MEASURE_CAPTURE")).Int64(); parseErr == nil && value > 0 {
			captureID = value
		}
	}

	if captureID == 0 {
		captures, listErr := sourceStore.ListCaptures()

		if listErr != nil {
			t.Fatal(listErr)
		}

		if len(captures) == 0 {
			t.Skip("no captures recorded")
		}

		captureID = captures[0].ID
	}
	startedAt, endedAt, err := sourceStore.Bounds(captureID)
	if err != nil {
		t.Fatal(err)
	}

	profileFrames, releaseProfiles, err := sourceStore.Frames(
		captureID,
		time.Time{},
	)

	if err != nil {
		t.Fatal(err)
	}

	depth := viper.GetInt("market.l3_depth")
	symbols, err := CaptureSymbolsFromStoredFrames(profileFrames, depth)
	releaseProfiles()

	if err != nil || len(symbols) == 0 {
		t.Fatalf("capture has no symbols: %v", err)
	}

	config := testtypes.NewScenarioConfig(symbols)
	config.StartTime = startedAt
	config.InitialBalances = map[string]float64{"USD": 200}
	config.Execution.DepthLevels = depth

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

	ctx := t.Context()
	market, err := NewMarketWithScenario(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer market.Close()

	market.WithAutoFill(config.Execution)

	uiChannel := transport.NewMapReduce[*types.UIFrame](nil, nil, nil)
	publicFeed, privateFeed := market.Feeds()
	thesis := types.NewThesis(ctx, uiChannel)
	replayStore, err := backtest.NewStore(filepath.Join(tmpDataPath, "symm.sqlite"))

	if err != nil {
		t.Fatal(err)
	}

	defer replayStore.Close()
	decisions, err := replayDecisions(replayStore)

	if err != nil {
		t.Fatal(err)
	}

	if len(decisions) != 0 {
		t.Fatalf("fresh replay store contains %d decisions", len(decisions))
	}

	recorder := &audit.Recorder{EventSink: replayStore.WriteEvent}
	previousUIAddr := viper.Get("ui.addr")
	viper.Set("ui.addr", "127.0.0.1:0")
	t.Cleanup(func() {
		viper.Set("ui.addr", previousUIAddr)
	})
	system := cmd.BootWithHub(
		ctx,
		thesis,
		publicFeed,
		privateFeed,
		uiChannel,
		nil,
		recorder,
	)

	if system == nil {
		t.Fatal("boot produced no system")
	}

	market.WithStagedReplay(thesis, system.Error)
	runErr := make(chan error, 1)

	go func() {
		runErr <- system.Run()
	}()

	defer func() {
		if err := system.Close(); err != nil {
			t.Errorf("close replay system: %v", err)
		}

		if err := <-runErr; err != nil {
			t.Errorf("run replay system: %v", err)
		}
	}()

	market.Drive(system)

	frames, release, err := sourceStore.Frames(captureID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	replayed := 0
	frameLimit := 0
	firstArrival := time.Time{}
	lastArrival := time.Time{}
	previousArrival := time.Time{}
	reachedEOF := false

	if os.Getenv("SYMM_MEASURE_LIMIT") != "" {
		if value, parseErr := json.Number(os.Getenv("SYMM_MEASURE_LIMIT")).Int64(); parseErr == nil && value > 0 {
			frameLimit = int(value)
		}
	}

	for {
		frame, ok := frames()
		if !ok {
			reachedEOF = true
			break
		}

		if err := waitReplayArrival(ctx, previousArrival, frame.ReceivedAt); err != nil {
			t.Fatal(err)
		}

		if err := market.ReplayFrame(frame); err != nil {
			t.Fatalf("replay frame %d: %v", replayed, err)
		}

		if replayed == 0 {
			firstArrival = frame.ReceivedAt
		}

		lastArrival = frame.ReceivedAt
		previousArrival = frame.ReceivedAt

		if os.Getenv("SYMM_MEASURE_TRACE") != "" && replayed < 20 {
			var header struct {
				Channel string `json:"channel"`
			}
			_ = json.Unmarshal(frame.Payload, &header)
			t.Logf(
				"TRACE frame %d %s/%s tick=%d",
				replayed,
				frame.Endpoint,
				header.Channel,
				system.Thesis.Tick,
			)
		}

		replayed++

		if frameLimit > 0 && replayed >= frameLimit {
			break
		}
	}
	release()

	if err := market.SettleReplay(); err != nil {
		t.Fatalf("settle replay: %v", err)
	}

	if !market.Public.Fence() {
		t.Fatal("public replay ingress fence failed")
	}

	if !market.Private.Fence() {
		t.Fatal("private replay ingress fence failed")
	}

	if !market.Level3.Fence() {
		t.Fatal("level3 replay ingress fence failed")
	}

	if err := system.Error(); err != nil {
		t.Fatalf("replay system failed before quiescence: %v", err)
	}

	if os.Getenv("SYMM_MEASURE_TRACE") != "" {
		for _, source := range types.WorkerSources {
			work := thesis.Work(source)
			t.Logf(
				"QUIESCENCE source=%s depth=%d idle=%t",
				source,
				work.Length(),
				work.Idle(),
			)
		}
	}

	if err := thesis.WaitForQuiescence(ctx); err != nil {
		t.Fatalf("wait for replay quiescence: %v", err)
	}

	if err := system.Error(); err != nil {
		t.Fatalf("replay system failed at quiescence: %v", err)
	}

	if replayed == 0 {
		t.Fatal("capture contains no replayable frames")
	}

	if !firstArrival.Equal(startedAt) {
		t.Fatalf("replay began at %s, capture begins at %s", firstArrival, startedAt)
	}

	if reachedEOF && !lastArrival.Equal(endedAt) {
		t.Fatalf("replay ended at %s, capture ends at %s", lastArrival, endedAt)
	}

	report := market.Report()
	tape := sqliteHindsightCeiling(
		t,
		sourceStore,
		captureID,
		replayed,
		firstArrival,
		lastArrival,
		replayStore,
	)
	realizedPct := report.Economics.NetPnL / 200 * 100

	t.Logf(
		"MEASURE capture %d (%s→%s): frames=%d net=%.6f realizedPct=%.4f%% gross=%.6f fees=%.6f filled=%d ceiling=%.6f%% sliceRealized=%.6f%% missed=%.6f%% legs=%d decisions=%d",
		captureID, startedAt.Format(time.RFC3339), endedAt.Format(time.RFC3339),
		replayed,
		report.Economics.NetPnL, realizedPct, report.Economics.GrossPnL, report.Economics.Fees,
		report.Mechanics.Filled,
		tape.Total.UpboundPct*100,
		tape.Total.RealizedPct*100,
		tape.Total.MissedPct*100,
		tape.Total.Legs,
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
					"MISSED %s buy=%s@%.6f sell=%s@%.6f profit=%.4f%% signalAt=%s action=%s thesis=%.4f confidence=%.4f direction=%.4f graph=%.4f threshold=%.4f opp=%v type=%s predictive=%v status=%q reason=%q cause=%q",
					symbol.Symbol,
					opportunity.Leg.BuyAt.Format("15:04:05"),
					opportunity.Leg.BuyPrice,
					opportunity.Leg.SellAt.Format("15:04:05"),
					opportunity.Leg.SellPrice,
					opportunity.Leg.ProfitPct*100,
					opportunity.Signal.At.Format("15:04:05"),
					opportunity.Signal.Action,
					opportunity.Signal.ThesisScore,
					opportunity.Signal.Confidence,
					opportunity.Signal.Direction,
					opportunity.Signal.GraphScore,
					opportunity.Signal.AdmissionThreshold,
					opportunity.Signal.Opportunity,
					opportunity.Signal.Type,
					opportunity.Signal.PredictiveReady,
					opportunity.Signal.PredictiveStatus,
					opportunity.Signal.Reason,
					opportunity.Signal.Cause,
				)
			}
		}
	}

	if os.Getenv("SYMM_MEASURE_DECISIONS") != "" {
		for _, decision := range tape.Decisions {
			t.Logf(
				"DECISION %s at=%s action=%s thesis=%.17g support=%.17g contradiction=%.17g conditions=%.17g confidence=%.17g direction=%.17g graph=%.17g threshold=%.17g opp=%v type=%s predictive=%v status=%q reason=%q cause=%q",
				decision.Symbol,
				decision.At.Format("15:04:05.000"),
				decision.Action,
				decision.ThesisScore,
				decision.ThesisSupport,
				decision.ThesisContradiction,
				decision.ThesisConditions,
				decision.Confidence,
				decision.Direction,
				decision.GraphScore,
				decision.AdmissionThreshold,
				decision.Opportunity,
				decision.OpportunityType,
				decision.PredictiveReady,
				decision.PredictiveStatus,
				decision.Reason,
				decision.Cause,
			)
		}
	}
}

func waitReplayArrival(
	ctx context.Context,
	previous time.Time,
	current time.Time,
) error {
	if current.IsZero() {
		return fmt.Errorf("replay frame has no arrival time")
	}

	if previous.IsZero() {
		return nil
	}

	if !current.After(previous) {
		return nil
	}

	delay := current.Sub(previous)

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
	replayed int,
	firstArrival time.Time,
	lastArrival time.Time,
	replayStore *backtest.Store,
) measuredTape {
	t.Helper()

	reducer := hindsight.NewReducer()
	frames, release, err := sourceStore.Frames(captureID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	scanned := 0
	scannedFirst := time.Time{}
	scannedLast := time.Time{}

	for scanned < replayed {
		frame, ok := frames()
		if !ok {
			break
		}

		if scanned == 0 {
			scannedFirst = frame.ReceivedAt
		}

		scannedLast = frame.ReceivedAt
		scanned++

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
	if scanned != replayed {
		t.Fatalf("hindsight scanned %d frames, replay consumed %d", scanned, replayed)
	}

	if !scannedFirst.Equal(firstArrival) || !scannedLast.Equal(lastArrival) {
		t.Fatalf(
			"hindsight slice %s→%s differs from replay %s→%s",
			scannedFirst,
			scannedLast,
			firstArrival,
			lastArrival,
		)
	}

	decisions, err := replayDecisions(replayStore)
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
replayDecisions reads every decision moment from the isolated replay store.
*/
func replayDecisions(store *backtest.Store) ([]hindsight.Decision, error) {
	next, err := store.Events()
	if err != nil {
		return nil, err
	}

	decisions := []hindsight.Decision{}

	for {
		kind, payload, _, ok := next()
		if !ok {
			break
		}

		if kind != "[]types.Decision" {
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

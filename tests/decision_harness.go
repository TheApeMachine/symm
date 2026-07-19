package tests

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
ExitKind is the full-exit cause a scenario expects from Stoploss.Regulate after
the mark/forecast phase. ExitNone means the lot must remain open.
*/
type ExitKind string

const (
	ExitTakeProfit ExitKind = "take_profit"
	ExitStop       ExitKind = "stop"
	ExitNone       ExitKind = "none"
)

/*
DecisionScenario is one market-simulation case for enter admission and optional
exit lock-in. Markets come from conditions.*; forecasts are seeded then applied
via CommitStrategy (strategy-given-forecast control surface) after Play warms
the Cut through Crypto.Tick.
*/
type DecisionScenario struct {
	Name             string
	Market           func() *Market
	Symbol           string
	EnterExpected    float64
	EnterUncertainty float64
	WantEnter        bool
	Exit             ExitKind
	PeakMul          float64
	BreachMul        float64
	ExitExpected     float64
	ExitUncertainty  float64
	WantCashLock     bool
}

/*
DecisionResult is the factual outcome of one scenario run for optimization
comparisons across tapes.
*/
type DecisionResult struct {
	Name           string
	Entered        bool
	ExitCause      string
	OpenAfter      int
	CashAfterEnter float64
	CashAfterExit  float64
	PeakReturn     float64
}

/*
DecisionResults is a batch of scenario outcomes for cause attribution.
*/
type DecisionResults []DecisionResult

/*
Causes counts Regulate exit Cause strings across results.
*/
func (results DecisionResults) Causes() map[string]int {
	counts := map[string]int{}

	for _, result := range results {
		cause := result.ExitCause

		if cause == "" {
			cause = "none"
		}

		counts[cause]++
	}

	return counts
}

/*
Run plays the market, admits (or rejects) an enter, then drives the configured
exit phase through Plan+Trade and paper fills.
*/
func (scenario DecisionScenario) Run(
	t testing.TB,
	signals SignalFactory,
) (DecisionResult, error) {
	t.Helper()

	result := DecisionResult{Name: scenario.Name}

	if scenario.Symbol == "" || scenario.Market == nil {
		return result, errnie.Err(
			errnie.Validation, "tests: decision scenario requires symbol and market", nil,
		)
	}

	session, err := NewSession(context.Background(), t, SessionOptions{
		Signals: signals,
	})

	if err != nil {
		return result, err
	}

	if _, err := session.Play(scenario.Market().Frames()); err != nil {
		return result, err
	}

	if err := session.SeedTakerFee(scenario.Symbol, 0.26); err != nil {
		return result, err
	}

	if err := session.SeedQuoteCapital(10_000); err != nil {
		return result, err
	}

	session.Desk.SetSlots(2, 2)
	thesis := types.NewThesis(nil, nil)
	SeedOpportunityForecast(
		thesis, scenario.Symbol, scenario.EnterExpected, scenario.EnterUncertainty,
	)
	SeedEarlyCognition(thesis, scenario.Symbol)

	if err := session.CommitStrategy(thesis); err != nil {
		return result, err
	}

	var enter *types.Decision

	for index := range thesis.Decisions {
		decision := &thesis.Decisions[index]

		if decision.Symbol == scenario.Symbol && decision.Action == types.ActionEnter {
			result.Entered = true
			enter = decision
			break
		}
	}

	if scenario.WantEnter != result.Entered {
		return result, errnie.Err(
			errnie.Validation, "tests: enter mismatch for "+scenario.Name, nil,
		)
	}

	if scenario.Exit == "" || !scenario.WantEnter {
		return result, nil
	}

	return scenario.exit(t, session, result, enter)
}

func (scenario DecisionScenario) exit(
	t testing.TB,
	session *Session,
	result DecisionResult,
	enter *types.Decision,
) (DecisionResult, error) {
	t.Helper()

	qty := 100.0

	if enter != nil && enter.ProposedQuantity != nil && enter.ProposedQuantity.Sign() > 0 {
		qty = enter.ProposedQuantity.Float64()
	}

	lot, statePath, err := session.SeedOpenLot(t, scenario.Symbol, qty, 0.02)

	if err != nil {
		return result, err
	}

	entry := lot.EntryPrice.Float64()
	result.CashAfterEnter = 10_000 - qty*entry

	if cash, cashErr := session.Balance.AvailableQuote(); cashErr == nil {
		result.CashAfterEnter = cash
	}

	peakMul := scenario.PeakMul

	if peakMul <= 0 {
		peakMul = 1.08
	}

	mark := entry * peakMul

	switch scenario.Exit {
	case ExitTakeProfit, ExitNone:
		lot.Stoploss.ObserveMark(mark)
	case ExitStop:
		breach := entry * 0.97

		if scenario.BreachMul > 0 {
			breach = entry * scenario.BreachMul
		}

		lot.Stoploss.ObserveMark(entry * peakMul)
		lot.Stoploss.ObserveMark(breach)
		mark = breach
	}

	if err := session.Mark(scenario.Symbol, mark); err != nil {
		return result, err
	}

	SetPaperPrice(t, statePath, scenario.Symbol, mark)
	exitThesis := types.NewThesis(nil, nil)
	exitUncertainty := scenario.ExitUncertainty

	if exitUncertainty <= 0 {
		exitUncertainty = 0.01
	}

	SeedOpportunityForecast(
		exitThesis, scenario.Symbol, scenario.ExitExpected, exitUncertainty,
	)

	if (scenario.Exit == ExitNone || scenario.Exit == ExitStop) &&
		len(exitThesis.Forecasts) > 0 {
		exitThesis.Forecasts[len(exitThesis.Forecasts)-1].IncrementalMSE =
			exitUncertainty * exitUncertainty * 0.01
	}

	exitThesis.At = time.Unix(1, 0).UTC()

	if err := session.CommitStrategy(exitThesis); err != nil {
		return result, err
	}

	for _, decision := range exitThesis.Decisions {
		if decision.Symbol == scenario.Symbol && decision.Action == types.ActionExit {
			result.ExitCause = decision.Cause
			break
		}
	}

	if live, liveErr := session.Balance.Holding(scenario.Symbol); liveErr == nil &&
		live.Stoploss != nil {
		result.PeakReturn = live.Stoploss.PeakReturn
	}

	result.OpenAfter = session.Desk.OpenPositions()

	if cash, cashErr := session.Balance.AvailableQuote(); cashErr == nil {
		result.CashAfterExit = cash
	}

	return scenario.check(result)
}

func (scenario DecisionScenario) check(result DecisionResult) (DecisionResult, error) {
	switch scenario.Exit {
	case ExitTakeProfit:
		if result.ExitCause != string(ExitTakeProfit) || result.OpenAfter != 0 {
			return result, errnie.Err(
				errnie.Validation,
				"tests: want take_profit close for "+scenario.Name+" got "+result.ExitCause,
				nil,
			)
		}
	case ExitStop:
		if result.ExitCause != string(ExitStop) || result.OpenAfter != 0 {
			return result, errnie.Err(
				errnie.Validation,
				"tests: want stop close for "+scenario.Name+" got "+result.ExitCause,
				nil,
			)
		}
	case ExitNone:
		if result.ExitCause != "" || result.OpenAfter != 1 {
			return result, errnie.Err(
				errnie.Validation,
				"tests: want hold for "+scenario.Name+" got "+result.ExitCause,
				nil,
			)
		}
	}

	if scenario.WantCashLock && result.CashAfterExit <= result.CashAfterEnter {
		return result, errnie.Err(
			errnie.Validation, "tests: cash not locked in "+scenario.Name, nil,
		)
	}

	return result, nil
}

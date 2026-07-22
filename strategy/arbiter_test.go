package strategy

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
TestArbiterSelect proves utility ordering plus the exact persistence, expiry,
and capacity effects of entry intent already selected on an earlier cut.
*/
func TestArbiterSelect(t *testing.T) {
	Convey("Given one free slot and two enter candidates", t, func() {
		previousNormal := viper.GetInt("trading.slots.normal")
		previousReserved := viper.GetInt("trading.slots.reserved")
		viper.Set("trading.slots.normal", 1)
		viper.Set("trading.slots.reserved", 0)
		Reset(func() {
			viper.Set("trading.slots.normal", previousNormal)
			viper.Set("trading.slots.reserved", previousReserved)
		})

		balance := broker.NewBalance(nil, nil, make(chan []byte, 1))
		desk := broker.NewDesk(nil, nil, nil, balance)
		rotate := NewRotate()
		admit := NewAdmit(context.Background(), balance, desk, rotate)
		arbiter := NewArbiter(desk, broker.NewPrice(nil), balance, admit, rotate)
		thesis := types.NewThesis(nil)
		thesis.Decisions = []types.Decision{
			{Action: types.ActionEnter, Symbol: "LOW", Utility: 0.1},
			{Action: types.ActionEnter, Symbol: "HIGH", Utility: 0.9},
		}

		arbiter.Select(thesis)

		var entered, rejected int

		for _, decision := range thesis.Decisions {
			switch decision.Action {
			case types.ActionEnter:
				entered++
				So(decision.Symbol, ShouldEqual, "HIGH")
			case types.ActionNothing:
				rejected++
				So(decision.Symbol, ShouldEqual, "LOW")
			}
		}

		So(entered, ShouldEqual, 1)
		So(rejected, ShouldEqual, 1)
	})

	scenarios := []struct {
		name                 string
		tick                 int64
		forecastPresent      bool
		sourceEpoch          uint64
		validThrough         uint64
		selectedAction       types.Action
		selectedCause        string
		selectedReason       string
		selectedPhase        string
		selectedHolding      bool
		challengerAction     types.Action
		challengerCause      string
		challengerReason     string
		challengerPhase      string
		challengerHolding    bool
		expectedLifecycleLen int
	}{
		{
			name:                 "unrelated large thesis tick with a prior source epoch",
			tick:                 1_000_000,
			forecastPresent:      true,
			sourceEpoch:          19,
			validThrough:         20,
			selectedAction:       types.ActionEnter,
			selectedPhase:        types.LifecycleEntrySelected,
			selectedHolding:      true,
			challengerAction:     types.ActionNothing,
			challengerCause:      "slots_full",
			challengerReason:     "normal slots full; reserved requires opportunity",
			expectedLifecycleLen: 1,
		},
		{
			name:                 "selected entry at the source epoch boundary",
			tick:                 200,
			forecastPresent:      true,
			sourceEpoch:          20,
			validThrough:         20,
			selectedAction:       types.ActionEnter,
			selectedPhase:        types.LifecycleEntrySelected,
			selectedHolding:      true,
			challengerAction:     types.ActionNothing,
			challengerCause:      "slots_full",
			challengerReason:     "normal slots full; reserved requires opportunity",
			expectedLifecycleLen: 1,
		},
		{
			name:                 "selected entry one source epoch past validity",
			tick:                 1,
			forecastPresent:      true,
			sourceEpoch:          21,
			validThrough:         20,
			selectedAction:       types.ActionNothing,
			selectedCause:        "entry_expired",
			selectedReason:       "selected forecast expired before a slot cleared",
			selectedPhase:        types.LifecycleExpired,
			challengerAction:     types.ActionEnter,
			challengerPhase:      types.LifecycleEntrySelected,
			challengerHolding:    true,
			expectedLifecycleLen: 2,
		},
		{
			name:                 "selected entry without a current forecast",
			tick:                 1_000_000,
			validThrough:         20,
			selectedAction:       types.ActionEnter,
			selectedPhase:        types.LifecycleEntrySelected,
			selectedHolding:      true,
			challengerAction:     types.ActionNothing,
			challengerCause:      "slots_full",
			challengerReason:     "normal slots full; reserved requires opportunity",
			expectedLifecycleLen: 1,
		},
	}

	for _, scenario := range scenarios {
		scenario := scenario

		Convey("Given a "+scenario.name, t, func() {
			previousNormal := viper.GetInt("trading.slots.normal")
			previousReserved := viper.GetInt("trading.slots.reserved")
			viper.Set("trading.slots.normal", 1)
			viper.Set("trading.slots.reserved", 0)
			Reset(func() {
				viper.Set("trading.slots.normal", previousNormal)
				viper.Set("trading.slots.reserved", previousReserved)
			})

			balance := broker.NewBalance(nil, nil, make(chan []byte, 1))
			desk := broker.NewDesk(nil, nil, nil, balance)
			rotate := NewRotate()
			admit := NewAdmit(context.Background(), balance, desk, rotate)
			arbiter := NewArbiter(
				desk, broker.NewPrice(nil), balance, admit, rotate,
			)
			at := time.Unix(scenario.tick, 0).UTC()
			selected := types.Decision{
				Action:            types.ActionEnter,
				Symbol:            "PENDING",
				At:                at,
				Utility:           0.8,
				ValidThroughEpoch: scenario.validThrough,
			}
			challenger := types.Decision{
				Action:            types.ActionEnter,
				Symbol:            "CHALLENGER",
				At:                at,
				Utility:           0.9,
				ValidThroughEpoch: uint64(scenario.tick + 1),
			}
			selectedHolding := &types.Holding{
				Symbol: "PENDING",
				Status: types.PENDING,
			}
			thesis := types.NewThesis(nil)
			thesis.Tick = scenario.tick
			thesis.At = at
			thesis.Decisions = []types.Decision{selected, challenger}

			if scenario.forecastPresent {
				thesis.Forecasts = []types.Forecasts{{
					Symbol:      selected.Symbol,
					SourceEpoch: scenario.sourceEpoch,
				}}
			}

			thesis.Holdings.Store(selected.Symbol, selectedHolding)
			thesis.Lifecycle.Store(
				selected.Symbol, types.LifecycleEntrySelected,
			)

			arbiter.Select(thesis)

			expectedSelected := selected
			expectedSelected.Action = scenario.selectedAction
			expectedSelected.Cause = scenario.selectedCause
			expectedSelected.Reason = scenario.selectedReason
			expectedChallenger := challenger
			expectedChallenger.Action = scenario.challengerAction
			expectedChallenger.Cause = scenario.challengerCause
			expectedChallenger.Reason = scenario.challengerReason

			if scenario.challengerAction == types.ActionNothing {
				expectedChallenger.Utility = 0
				expectedChallenger.Alternatives = map[string]float64{}
			}

			So(thesis.Decisions, ShouldResemble, []types.Decision{
				expectedSelected,
				expectedChallenger,
			})
			phase, phaseFound := thesis.Lifecycle.Load(selected.Symbol)
			So(phaseFound, ShouldBeTrue)
			So(phase, ShouldEqual, scenario.selectedPhase)
			challengerPhase, challengerPhaseFound := thesis.Lifecycle.Load(
				challenger.Symbol,
			)
			So(challengerPhaseFound, ShouldEqual, scenario.challengerPhase != "")

			if challengerPhaseFound {
				So(challengerPhase, ShouldEqual, scenario.challengerPhase)
			}

			storedSelected, selectedFound := thesis.Holdings.Load(selected.Symbol)
			So(selectedFound, ShouldEqual, scenario.selectedHolding)

			if selectedFound {
				So(storedSelected, ShouldEqual, selectedHolding)
			}

			storedChallenger, challengerFound := thesis.Holdings.Load(
				challenger.Symbol,
			)
			So(challengerFound, ShouldEqual, scenario.challengerHolding)

			if challengerFound {
				holding := storedChallenger.(*types.Holding)
				So(holding.Symbol, ShouldEqual, challenger.Symbol)
				So(holding.Status, ShouldEqual, types.PENDING)
				So(holding.Qty, ShouldBeNil)
				So(holding.IsOpportunity, ShouldBeFalse)
				So(holding.Stoploss, ShouldNotBeNil)
			}

			lifecycleCount := 0
			thesis.Lifecycle.Range(func(_, _ any) bool {
				lifecycleCount++
				return true
			})
			So(lifecycleCount, ShouldEqual, scenario.expectedLifecycleLen)
		})
	}
}

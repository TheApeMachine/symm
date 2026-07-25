package logic_test

import (
	"fmt"
	"testing"

	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func TestProbeFullAnalyzerPath(t *testing.T) {
	proofs := []struct {
		name    string
		state   tests.MarketState
		prepare bool
	}{
		{"baseline", tests.MarketStateBaseline, false},
		{"fast pump", tests.MarketStateFastPump, false},
		{"slow pump", tests.MarketStateSlowPump, false},
		{"fast dump", tests.MarketStateFastDump, false},
		{"slow dump", tests.MarketStateSlowDump, false},
		{"volume absorption", tests.MarketStateVolumeAbsorption, false},
		{"persistent adverse divergence", tests.MarketStateAdverseDivergence, true},
		{"leader follower", tests.MarketStateLeaderFollower, false},
	}

	for _, proof := range proofs {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		if err != nil {
			t.Fatalf("%s boot: %v", proof.name, err)
		}
		if err := market.Warmup(tests.Idle); err != nil {
			t.Fatalf("%s warm: %v", proof.name, err)
		}
		if proof.prepare {
			if err := market.Transition(proof.state, tests.Idle); err != nil {
				t.Fatalf("%s prep: %v", proof.name, err)
			}
		}
		types.SetFocus(market.Symbols[0])
		var advanced, replayed, cuts, categories int
		var cognitions int
		if err := market.Transition(proof.state, func() error {
			cuts++
			thesis := wired.Thesis
			thesis.Manifold.Range(func(_, value any) bool {
				state := value.(manifold.State)
				if state.Replay {
					replayed++
				} else {
					advanced++
				}
				return true
			})
			thesis.Cognition.Range(func(_, _ any) bool {
				cognitions++
				return true
			})
			categories += len(thesis.Categories)
			return nil
		}); err != nil {
			t.Fatalf("%s transition: %v", proof.name, err)
		}
		res := len(wired.Thesis.Resonance) > 0
		man := false
		wired.Thesis.Manifold.Range(func(_, value any) bool {
			state, ok := value.(manifold.State)
			man = ok && state.GasReady()
			return !man
		})
		cog := false
		wired.Thesis.Cognition.Range(func(_, _ any) bool {
			cog = true
			return false
		})
		fmt.Printf("%s: cuts=%d advanced=%d replayed=%d cognitions=%d categories=%d frames=%v/%v/%v\n",
			proof.name, cuts, advanced, replayed, cognitions, categories, res, man, cog)
		types.SetFocus("")
		_ = wired.Close()
		market.Close()
	}
}

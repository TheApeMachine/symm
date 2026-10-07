package strategy

import (
	"bytes"
	"math"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	"golang.design/x/lockfree/lf"
)

/*
paper trades the live market with the trained Model through the Desk and
refines the Model on every realized round trip. It owns each symbol's live
episode (its rolling token window and the contexts that entered and exited
the open position) and the realized score.
*/
type paper struct {
	desk        *broker.Desk
	model       *Model
	reporter    *Reporter
	episodes    *lf.OrderedMap[string, *episode]
	resolved    atomic.Int64
	wins        atomic.Int64
	returnsBits atomic.Uint64
}

func newPaper(desk *broker.Desk, model *Model, reporter *Reporter) *paper {
	paper := &paper{
		desk:     desk,
		model:    model,
		reporter: reporter,
		episodes: lf.NewOrderedMap[string, *episode](func(left, right string) bool {
			return left < right
		}),
	}

	desk.OnClose(paper.settle)
	return paper
}

func (paper *paper) log(format string, args ...any) {
	if paper != nil && paper.reporter != nil {
		paper.reporter.Log(format, args...)
	}
}

/*
episodeState holds one symbol's live state.
*/
type episodeState struct {
	window [][]byte
	entry  []byte
	exit   []byte
	mark   float64
	marked bool
}

/*
episode is one symbol's lock-free live state.
*/
type episode struct {
	state atomic.Pointer[episodeState]
}

/*
update applies change to a copy of the current state and publishes it.
*/
func (episode *episode) update(change func(*episodeState)) *episodeState {
	for {
		oldState := episode.state.Load()
		newState := *oldState
		change(&newState)

		if episode.state.CompareAndSwap(oldState, &newState) {
			return oldState
		}
	}
}

/*
trade appends the symbol's region token to its window, asks the Model for
the action on that window, and acts on it when the Desk allows: enter when
flat (and the solvers do not veto), exit when holding.
*/
func (paper *paper) trade(prior *data.Measurement, tok []byte, snapshot *ReportSnapshot) {
	symbol := prior.Label

	span, err := paper.model.Count("span")

	if err != nil {
		errnie.Error(err)
		return
	}

	current := paper.episode(symbol)
	var question []byte

	current.update(func(state *episodeState) {
		window := state.window

		if len(window) == 0 || !bytes.Equal(window[len(window)-1], tok) {
			window = append(append(make([][]byte, 0, len(window)+1), window...), tok)
		}

		if limit := int(span); limit >= 1 && len(window) > limit {
			window = window[len(window)-limit:]
		}

		state.window = window
		question = bytes.Join(window, []byte("/"))
	})

	call, err := paper.model.Recall(string(question), "")

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "[paper] unable to evaluate "+symbol, err))
		return
	}

	snapshot.Confidence = call.Confidence
	snapshot.Contrast = call.Contrast

	state := paper.desk.State(symbol)

	if state == broker.HOLDING {
		paper.mark(symbol)
	}

	if state == broker.FLAT && call.Winner == actionEnter && authorized(
		solverMeasurement(prior, "resonance"), solverMeasurement(prior, "manifold"),
	) {
		paper.act(symbol, paper.desk.Enter, func(state *episodeState, context []byte) {
			state.entry = context
		}, question)
		snapshot.Action = 1
		paper.log("[PAPER:ENTER] symbol=%s context=%s confidence=%.3f contrast=%.3f",
			symbol, string(question), call.Confidence, call.Contrast,
		)
	}

	if state == broker.HOLDING && call.Winner == actionExit {
		paper.act(symbol, paper.desk.Exit, func(state *episodeState, context []byte) {
			state.exit = context
		}, question)
		snapshot.Action = 2
		paper.log("[PAPER:EXIT] symbol=%s context=%s confidence=%.3f contrast=%.3f",
			symbol, string(question), call.Confidence, call.Contrast,
		)
	}
}

/*
solverMeasurement locates a solver producer's measurement in prior or its
peers.
*/
func solverMeasurement(prior *data.Measurement, source string) *data.Measurement {
	if prior.Source == source {
		return prior
	}

	for _, peer := range prior.Peers() {
		if peer != nil && peer.Source == source {
			return peer
		}
	}

	return nil
}

/*
authorized applies the execution triad gate:
 1. Resonance: Verifies presence of structural shock / dislocation (Surprise > 0),
    vetoing entries during predictable equilibrium churn.
 2. Manifold: Verifies the order book medium permits wave propagation, vetoing
    entries if resting orders exhibit locked synchronization opposing the move.
*/
func authorized(resonanceM, manifoldM *data.Measurement) bool {
	if resonanceM != nil {
		surprise, err := readMetric(resonanceM, "surprise")

		if err == nil && surprise != nil && surprise.Raw <= 0 {
			return false
		}
	}

	if manifoldM == nil {
		return true
	}

	kuramoto, err := readMetric(manifoldM, "kuramoto_r")

	if err != nil || kuramoto == nil || kuramoto.Raw < 1.0 {
		return true
	}

	pressure, err := readMetric(manifoldM, "pressure_grad_norm")

	if err == nil && pressure != nil && pressure.Raw > 0 {
		return false
	}

	return true
}

/*
act records the context that triggers a Desk operation before submitting it,
because the resulting fill may settle before the operation returns. A
rejected operation withdraws the context.
*/
func (paper *paper) act(
	symbol string,
	operation func(string) error,
	record func(*episodeState, []byte),
	question []byte,
) {
	held := paper.episode(symbol)
	held.update(func(state *episodeState) { record(state, question) })

	err := operation(symbol)

	if err == nil {
		return
	}

	errnie.Error(err)
	held.update(func(state *episodeState) { record(state, nil) })
}

/*
mark records the open position's unrealized return for the UI score.
Choppy marks never train the trie; only realized closures do.
*/
func (paper *paper) mark(symbol string) {
	pnl, basis, err := paper.desk.Unrealized(symbol)

	if err != nil {
		errnie.Error(err)
		return
	}

	mark := pnl.SetScale(decimal.DefaultScale).Div(basis).Float64()

	paper.episode(symbol).update(func(state *episodeState) {
		state.mark = mark
		state.marked = true
	})
}

/*
settle consumes one realized round trip from the Desk and refines the trie
on the contexts that entered and exited it. A winning entry reinforces enter
with its return and teaches exit on the exit context with the same return —
enter and exit are always taught together (TRAINING.md: exit only if we have
entered). A losing entry dampens enter on the entry context (negative
feedback); wait is abstention, never a leaf. Losses never teach exit alone.
*/
func (paper *paper) settle(closure broker.Closure) {
	feedback := closure.Realized.SetScale(decimal.DefaultScale).Div(closure.Cost).Float64()

	settled := paper.episode(closure.Symbol).update(func(state *episodeState) {
		state.entry = nil
		state.exit = nil
		state.marked = false
	})

	paper.resolved.Add(1)
	paper.addReturn(feedback)

	if feedback > 0 {
		paper.wins.Add(1)
	}

	resolvedCount := paper.resolved.Load()
	winsCount := paper.wins.Load()
	winRate := 0.0

	if resolvedCount > 0 {
		winRate = float64(winsCount) / float64(resolvedCount) * 100
	}

	paper.log(
		"[PAPER:SETTLE] symbol=%s realized=%s cost=%s return=%+.4f%% (resolved=%d wins=%d win_rate=%.1f%%)",
		closure.Symbol, closure.Realized.String(), closure.Cost.String(), feedback*100,
		resolvedCount, winsCount, winRate,
	)

	if settled.entry == nil {
		errnie.Error(errnie.Err(
			errnie.Conflict,
			"[paper] closed position has no entry context: "+closure.Symbol,
			nil,
		))

		return
	}

	if feedback <= 0 {
		loss := -feedback

		if loss == 0 {
			loss = 1e-6
		}

		// Dampen the losing enter sequence; wait is not a terminal action.
		if err := paper.model.Teach(string(settled.entry), actionEnter, -loss); err != nil {
			errnie.Error(err)
		}

		return
	}

	if err := paper.model.Teach(string(settled.entry), actionEnter, feedback); err != nil {
		errnie.Error(err)
		return
	}

	if settled.exit == nil {
		errnie.Error(errnie.Err(
			errnie.Conflict,
			"[paper] winning round trip taught enter without exit context: "+closure.Symbol,
			nil,
		))

		return
	}

	if err := paper.model.Teach(string(settled.exit), actionExit, feedback); err != nil {
		errnie.Error(err)
	}
}

func (paper *paper) addReturn(value float64) {
	for {
		oldBits := paper.returnsBits.Load()
		newBits := math.Float64bits(math.Float64frombits(oldBits) + value)

		if paper.returnsBits.CompareAndSwap(oldBits, newBits) {
			return
		}
	}
}

/*
score reports win rate and edge over realized round trips plus the current
marks of open positions. It is display state, not training feedback.
*/
func (paper *paper) score() (int64, float64, float64) {
	count := paper.resolved.Load()
	wins := paper.wins.Load()
	returns := math.Float64frombits(paper.returnsBits.Load())

	paper.episodes.Range("", "\xff\xff\xff\xff", func(_ string, held *episode) {
		state := held.state.Load()

		if !state.marked {
			return
		}

		count++
		returns += state.mark

		if state.mark > 0 {
			wins++
		}
	})

	if count > 0 {
		return count, float64(wins) / float64(count), returns / float64(count)
	}

	if paper.desk.Balance == nil {
		return 0, 0, 0
	}

	snap := paper.desk.Balance.Snapshot()

	if snap == nil || snap.Unrealized == nil || snap.Equity == nil {
		return 0, 0, 0
	}

	base := snap.Equity.Sub(snap.Unrealized)

	if base.Sign() <= 0 || snap.Unrealized.Sign() == 0 {
		return 0, 0, 0
	}

	ret := snap.Unrealized.SetScale(decimal.DefaultScale).Div(base).Float64()

	if ret > 0 {
		return 1, 1, ret
	}

	return 1, 0, ret
}

/*
episode returns the symbol's live state.
*/
func (paper *paper) episode(symbol string) *episode {
	if held, ok := paper.episodes.Get(symbol); ok && held != nil {
		return held
	}

	held := &episode{}
	held.state.Store(&episodeState{})
	paper.episodes.Set(symbol, held)
	actual, _ := paper.episodes.Get(symbol)
	return actual
}

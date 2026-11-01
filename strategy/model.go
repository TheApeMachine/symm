package strategy

import (
	"context"
	"strconv"
	"sync/atomic"

	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

type Model struct {
	*runtime.System
	desk       *broker.Desk
	price      *broker.Price
	store      *store.Client
	tree       *atomic.Pointer[Tree]
	Confidence *Confidence
	resetPath  func(string)
}

func NewModel(
	ctx context.Context,
	desk *broker.Desk,
	price *broker.Price,
	resetPath func(string),
) *Model {
	client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

	model := &Model{
		System:     runtime.NewSystem(ctx, "model"),
		desk:       desk,
		price:      price,
		store:      client,
		tree:       &atomic.Pointer[Tree]{},
		Confidence: NewConfidence(ctx),
		resetPath:  resetPath,
	}

	tree := NewTree(ctx, "model/paths/")
	model.tree.Store(tree)
	model.Confidence.Publish()

	return model
}

func (model *Model) Step(envelope *runtime.Envelope) *runtime.Envelope {
	if envelope == nil {
		return nil
	}

	// Settled lots must hit the tree even when the symbol drops off this tick's
	// path map — otherwise Exit never Inserts and model/paths stays empty.
	model.flushPending(envelope)

	for symbol, cut := range envelope.Paths {
		if cut == nil {
			continue
		}

		path := cut.String()

		rez := envelope.Resonance[symbol]
		learn := system.Cfg.Learning
		bypass := learn.ResonanceBypass

		if rez == nil {
			if !bypass {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "no resonance reading",
					Gate:   "resonance",
					Path:   path,
				})

				continue
			}

			rez = model.bypassReading(learn)
		}

		surprise := rez["surprise"]
		supported := int(rez["supported_horizon"])
		confidence, hasPrecision := rez["task_relative_precision"]
		minHorizon := learn.MinSupportedHorizon
		minPrecision := learn.MinTaskRelativePrecision
		minSurprise := learn.MinSurprise

		if !bypass && (rez["calibrated"] < 1 || supported < minHorizon || !hasPrecision || confidence < minPrecision || surprise < minSurprise) {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     model.gateReason(rez, supported, hasPrecision, confidence, surprise, minHorizon, minPrecision, minSurprise),
				Gate:       "resonance",
				Path:       path,
				Confidence: confidence,
			})

			continue
		}

		curve := make([]float64, 0, supported)

		for i := range supported {
			value, ok := rez["forward_curve_"+strconv.Itoa(i)]

			if !ok {
				break
			}

			curve = append(curve, value)
		}

		enterNet, holdNet, exitNet, risk, ok := resonance.ForwardStoryNets(curve)

		if !ok {
			if !bypass {
				envelope.Decide(&runtime.Decision{
					Symbol:     symbol,
					Action:     "skip",
					Reason:     "forward curve incomplete",
					Gate:       "resonance",
					Path:       path,
					Confidence: confidence,
				})

				continue
			}

			enterNet, holdNet, exitNet, risk = 1, 1, 0, 0
		}

		lots := model.desk.Positions(symbol)
		open := make([]*broker.Position, 0, len(lots))

		for _, lot := range lots {
			if lot != nil && !lot.IsClosed() {
				open = append(open, lot)
			}
		}

		regime := FlatRegime()

		if len(open) > 0 {
			regime = OpenRegime()
		}

		action, resistance, disagreed := model.tree.Load().Predict(*cut, regime)

		minEnter := learn.MinEnterZ

		bullish := bypass || (enterNet >= minEnter &&
			holdNet >= minEnter &&
			exitNet <= holdNet &&
			risk < minEnter)

		if disagreed {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "tree leaves disagree under cut",
				Gate:       "tree",
				Path:       path,
				Confidence: confidence,
			})

			continue
		}

		// Concurrent books: enter even when another book already holds the symbol.
		wantEnter := action == ActionEnter ||
			(action == ActionNone && bullish && cut.Len() > 0)

		if wantEnter && !Board(envelope.Regions[symbol]).Tradable() {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "oracle veto lit — not tradable",
				Gate:       "oracle",
				Path:       path,
				Confidence: confidence,
			})

			continue
		}

		if action != ActionNone {
			model.applyAction(
				envelope, symbol, path, action, resistance,
				bullish, bypass, open, confidence,
			)

			continue
		}

		minPath := learn.MinPathLength

		if len(open) > 0 && path != "" && action != ActionExit {
			if cut.Len() >= minPath {
				for _, lot := range open {
					model.Hold(lot, path)
				}

				envelope.Decide(&runtime.Decision{
					Symbol:     symbol,
					Action:     "hold",
					Reason:     "open lot tracking live path",
					Path:       path,
					Confidence: confidence,
				})

				continue
			}
		}

		if cut.Len() > 0 && len(open) == 0 && !bullish {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "forward story not bullish",
				Gate:       "story",
				Path:       path,
				Confidence: confidence,
			})

			continue
		}

		if cut.Len() < minPath {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "path shorter than min length",
				Gate:       "path",
				Path:       path,
				Confidence: confidence,
			})

			continue
		}

		if wantEnter && action == ActionNone {
			model.exploreEnter(envelope, symbol, path, confidence)
		}
	}

	return envelope
}

func (model *Model) applyAction(
	envelope *runtime.Envelope,
	symbol, path string,
	action Action,
	resistance float64,
	bullish, bypass bool,
	open []*broker.Position,
	confidence float64,
) {
	switch action {
	case ActionEnter:
		if !bullish {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "enter leaf but forward story not bullish",
				Gate:       "story",
				Path:       path,
				Confidence: confidence,
			})

			return
		}

		model.enterTiers(envelope, symbol, path, resistance, bypass, confidence)

	case ActionExit:
		if !bullish && len(open) > 0 {
			for _, lot := range open {
				model.Exit(lot, path)
			}

			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "exit",
				Reason:     "tree exit · story no longer bullish",
				Path:       path,
				Confidence: confidence,
			})
		} else {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "exit leaf without open lot or still bullish",
				Gate:       "story",
				Path:       path,
				Confidence: confidence,
			})
		}

		model.resetPath(symbol)

	case ActionHold:
		if len(open) > 0 {
			for _, lot := range open {
				model.Hold(lot, path)
			}

			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "hold",
				Reason:     "tree hold",
				Path:       path,
				Confidence: confidence,
			})
		} else {
			envelope.Decide(&runtime.Decision{
				Symbol:     symbol,
				Action:     "skip",
				Reason:     "hold leaf with no open lot",
				Gate:       "position",
				Path:       path,
				Confidence: confidence,
			})
		}

		model.resetPath(symbol)
	}
}

/*
enterTiers routes a validated enter leaf onto virtual, paper, and optionally
real books according to resistance and confidence.
*/
func (model *Model) enterTiers(
	envelope *runtime.Envelope,
	symbol, path string,
	resistance float64,
	bypass bool,
	confidence float64,
) {
	eligible := PaperEligible(resistance)

	// High resistance (or lab bypass) keeps exploring on the virtual book.
	if bypass || !eligible {
		model.tryEnter(
			envelope, symbol, path, broker.LedgerVirtual, false,
			"tree enter · virtual sample", confidence,
		)

		return
	}

	// Validated path: paper always; real when confidence is armed.
	opened := make([]string, 0, 2)

	if model.openLedger(envelope, symbol, path, broker.LedgerPaper, true, confidence) {
		opened = append(opened, "paper")
	}

	if model.Confidence.Armed() {
		if model.openLedger(envelope, symbol, path, broker.LedgerReal, true, confidence) {
			opened = append(opened, "real")
		}
	} else if RealCeilingAllowed() {
		envelope.Decide(&runtime.Decision{
			Symbol:     symbol,
			Action:     "skip",
			Reason:     "real halted · paper edge not armed",
			Gate:       "confidence",
			Path:       path,
			Book:       string(TierReal),
			Confidence: confidence,
		})
	}

	if len(opened) == 0 {
		return
	}

	book := opened[len(opened)-1]
	envelope.Decide(&runtime.Decision{
		Symbol:     symbol,
		Action:     "enter",
		Reason:     "tree enter · " + joinBooks(opened),
		Path:       path,
		Book:       book,
		Virtual:    false,
		Confidence: confidence,
	})
}

func joinBooks(books []string) string {
	switch len(books) {
	case 0:
		return ""
	case 1:
		return books[0]
	default:
		out := books[0]
		for _, book := range books[1:] {
			out += "+" + book
		}
		return out
	}
}

func (model *Model) openLedger(
	envelope *runtime.Envelope,
	symbol, path string,
	ledger broker.Ledger,
	pathFound bool,
	confidence float64,
) bool {
	if err := model.desk.Enter(symbol, path, ledger, pathFound); err != nil {
		gate, why := "desk", err.Error()

		if denial, ok := err.(*broker.EntryDenial); ok && denial != nil {
			gate, why = denial.Gate, denial.Reason
		}

		if gate == "slot" {
			return false
		}

		envelope.Decide(&runtime.Decision{
			Symbol:     symbol,
			Action:     "blocked",
			Reason:     why,
			Gate:       gate,
			Path:       path,
			Book:       ledger.String(),
			Virtual:    ledger.Virtual(),
			Confidence: confidence,
		})

		return false
	}

	return true
}

func (model *Model) tryEnter(
	envelope *runtime.Envelope,
	symbol, path string,
	ledger broker.Ledger,
	pathFound bool,
	reason string,
	confidence float64,
) {
	if err := model.desk.Enter(symbol, path, ledger, pathFound); err != nil {
		gate, why := "desk", err.Error()

		if denial, ok := err.(*broker.EntryDenial); ok && denial != nil {
			gate, why = denial.Gate, denial.Reason
		}

		// Slot already filled on this book is quiet — another lot is refining.
		if gate == "slot" {
			return
		}

		envelope.Decide(&runtime.Decision{
			Symbol:     symbol,
			Action:     "blocked",
			Reason:     why,
			Gate:       gate,
			Path:       path,
			Book:       ledger.String(),
			Virtual:    ledger.Virtual(),
			Confidence: confidence,
		})

		return
	}

	envelope.Decide(&runtime.Decision{
		Symbol:     symbol,
		Action:     "enter",
		Reason:     reason,
		Path:       path,
		Book:       ledger.String(),
		Virtual:    ledger.Virtual(),
		Confidence: confidence,
	})
}

func (model *Model) gateReason(
	rez map[string]float64,
	supported int,
	hasPrecision bool,
	confidence, surprise float64,
	minHorizon int,
	minPrecision, minSurprise float64,
) string {
	if rez["calibrated"] < 1 {
		return "resonance not calibrated"
	}

	if supported < minHorizon {
		return "supported horizon below minimum"
	}

	if !hasPrecision {
		return "task-relative precision not ready"
	}

	if confidence < minPrecision {
		return "task-relative precision below threshold"
	}

	if surprise < minSurprise {
		return "surprise below threshold"
	}

	return "resonance gate"
}

/*
bypassReading invents a calibrated one-step curve so lab bypass can exercise
story/tree/desk without a live manifold.
*/
func (model *Model) bypassReading(learn *system.Learning) map[string]float64 {
	horizon := len(resonance.Features)

	if learn.MinSupportedHorizon > horizon {
		horizon = learn.MinSupportedHorizon
	}

	rez := map[string]float64{
		"calibrated":              1,
		"surprise":                1,
		"task_relative_precision": 1,
		"supported_horizon":       float64(horizon),
	}

	// Enter/hold det+sup and every oracle approve dim; vetoes stay flat.
	for i := range horizon {
		val := 0.0

		switch {
		case i == 0 || i == 1 || i == 6 || i == 7:
			val = 1
		case i >= 9 && i%2 == 0:
			val = 1
		}

		rez["forward_curve_"+strconv.Itoa(i)] = val
	}

	return rez
}

/*
exploreEnter opens a cold-tree virtual sample when story (or bypass) clears
and no enter leaf exists yet — otherwise the tree never seeds.
*/
func (model *Model) exploreEnter(
	envelope *runtime.Envelope,
	symbol, path string,
	confidence float64,
) {
	model.tryEnter(
		envelope, symbol, path, broker.LedgerVirtual, false,
		"cold-tree explore · virtual sample", confidence,
	)
}

func (model *Model) flushPending(envelope *runtime.Envelope) {
	for _, key := range model.desk.Pending() {
		_, symbol := broker.ParsePendingKey(key)
		path := model.exitCut(envelope, symbol, nil)

		// Wait for a live regime cut distinct from entry before consuming
		// the pending settle — never invent exit leaves or reuse entry_path.
		if path == "" {
			continue
		}

		pending, ok := model.desk.TakePending(key)
		if !ok || pending == nil {
			continue
		}

		path = model.exitCut(envelope, symbol, pending)
		if path == "" {
			model.desk.PendingPut(key, pending)
			continue
		}

		model.Exit(pending, path)
	}
}

/*
exitCut picks a regime path for the exit leaf. Empty means wait.
*/
func (model *Model) exitCut(
	envelope *runtime.Envelope,
	symbol string,
	pending *broker.Position,
) string {
	path := ""

	if envelope != nil {
		if cut := envelope.Paths[symbol]; cut != nil {
			path = cut.String()
		}
	}

	if path == "" && pending != nil {
		path = pending.ExitPath
	}

	if path == "" && pending != nil {
		if n := len(pending.HoldPaths); n > 0 {
			path = pending.HoldPaths[n-1]
		}
	}

	if pending != nil && (path == "" || path == pending.EntryPath) {
		return ""
	}

	return path
}

func (model *Model) Hold(position *broker.Position, path string) {
	if position == nil || path == "" || path == position.EntryPath {
		return
	}

	// One live cut only — a diary of every board flicker is not a policy.
	if len(position.HoldPaths) == 1 && position.HoldPaths[0] == path {
		return
	}

	position.HoldPaths = []string{path}
}

func (model *Model) Exit(
	position *broker.Position, path string,
) {
	if position == nil || path == "" || path == position.EntryPath {
		return
	}

	position.ExitPath = path

	tree := model.tree.Load()

	// Virtual episode = enter + exit. Hold paths are live tracking, not leaves.
	// Ensure seeds new leaves at 0.5 once; never Insert-overwrite on re-settle
	// or resistance resets to ~0 and paper eligibility opens immediately.
	if position.Ledger().Virtual() {
		tree.Ensure(position.EntryPath, ActionEnter)
		tree.Ensure(position.ExitPath, ActionExit)
	}

	resistance := 0.05

	if position.PnL != nil && position.PnL.Sign() > 0 {
		resistance = -0.05
	}

	tree.Update(position.EntryPath, resistance)
	tree.Update(position.ExitPath, resistance)
	model.Confidence.Record(position)
}

package optimizer

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public/response"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/trader"
)

type ReplayEvaluator struct {
	frames  []ReplayFrame
	options Options
}

func NewReplayEvaluator(frames []ReplayFrame, options Options) *ReplayEvaluator {
	return &ReplayEvaluator{
		frames:  append([]ReplayFrame(nil), frames...),
		options: normalizeOptions(options),
	}
}

func (evaluator *ReplayEvaluator) Evaluate(treeYAML []byte) (ReplayResult, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree := dmt.NewTree("")
	playbook, err := logic.NewTreeFromBytes(ctx, pool, treeYAML)
	if err != nil {
		return ReplayResult{}, err
	}

	crypto, err := trader.NewCrypto(ctx, pool, tree)
	if err != nil {
		return ReplayResult{}, err
	}
	defer crypto.Close()
	crypto.SuppressReplaySideEffects()
	crypto.SetReplayPlaybook(playbook)

	ledger := newLedger(evaluator.options)
	crypto.PrepareReplay(ledger.BalancesArtifact())
	handler := response.NewTreeHandlerWithoutCapture(tree)
	prices := make(map[string]float64)

	for _, frame := range evaluator.frames {
		for _, artifact := range frame.Artifacts {
			handler.Send(replayArtifact(artifact))
		}
		for symbol, price := range frame.Prices {
			if price > 0 && !math.IsNaN(price) && !math.IsInf(price, 0) {
				prices[symbol] = price
			}
		}

		ledger.Mark(prices)
		crypto.SetReplayBalances(ledger.BalancesArtifact())
		result := crypto.ReplayCandidateTick()
		ledger.Apply(result.Actions, prices)
	}

	return ledger.Result(evaluator.frames), nil
}

func replayArtifact(frame ReplayArtifact) *datura.Artifact {
	origin := strings.TrimSpace(frame.Origin)
	if origin == "" {
		origin = "kraken:public"
	}

	payload := datura.Map[any](frame.Payload)
	if payload == nil {
		payload = datura.Map[any]{}
	}
	if frame.Role != "" && payload["channel"] == nil {
		payload["channel"] = frame.Role
	}
	if frame.Type != "" && payload["type"] == nil {
		payload["type"] = frame.Type
	}

	artifact := datura.Acquire(origin, datura.APPJSON).
		WithRole(frame.Role).
		WithScope(frame.Scope).
		WithPayload(payload.Marshal())

	if frame.Timestamp > 0 {
		artifact.SetTimestamp(frame.Timestamp)
	}

	return artifact
}

type position struct {
	symbol string
	qty    float64
	entry  float64
	mark   float64
}

type replayLedger struct {
	options     Options
	cash        float64
	start       float64
	positions   map[string]position
	trades      int
	peakWallet  float64
	maxDrawdown float64
}

func newLedger(options Options) *replayLedger {
	options = normalizeOptions(options)
	return &replayLedger{
		options:    options,
		cash:       options.InitialCash,
		start:      options.InitialCash,
		positions:  make(map[string]position),
		peakWallet: options.InitialCash,
	}
}

func (ledger *replayLedger) Apply(actions []*datura.Artifact, prices map[string]float64) {
	if len(actions) == 0 {
		return
	}

	sort.SliceStable(actions, func(first, second int) bool {
		firstExit := replayActionIsExit(actions[first])
		secondExit := replayActionIsExit(actions[second])
		if firstExit != secondExit {
			return firstExit
		}

		return datura.Peek[float64](actions[first], "entry_confidence") >
			datura.Peek[float64](actions[second], "entry_confidence")
	})

	for _, action := range actions {
		if action == nil {
			continue
		}

		symbol, _ := action.Scope()
		if symbol == "" {
			symbol = datura.Peek[string](action, "symbol")
		}
		price := prices[symbol]
		if symbol == "" || price <= 0 {
			continue
		}

		side := logic.Side(datura.Peek[string](action, "side"))
		if side == "" {
			role, _ := action.Role()
			side = logic.Side(role)
		}
		actionType := logic.ActionType(datura.Peek[string](action, "type"))
		feeRate := ledger.feeRate(actionType)

		if side == logic.SideSell || actionType.IsExit() {
			ledger.close(symbol, price, feeRate)
			continue
		}
		if side != logic.SideBuy || actionType.IsExit() {
			continue
		}

		ledger.open(symbol, price, feeRate)
	}
}

func replayActionIsExit(action *datura.Artifact) bool {
	if action == nil {
		return false
	}
	side := logic.Side(datura.Peek[string](action, "side"))
	if side == "" {
		role, _ := action.Role()
		side = logic.Side(role)
	}
	actionType := logic.ActionType(datura.Peek[string](action, "type"))

	return side == logic.SideSell || actionType.IsExit()
}

func (ledger *replayLedger) open(symbol string, price float64, feeRate float64) {
	if _, exists := ledger.positions[symbol]; exists {
		return
	}
	if len(ledger.positions) >= ledger.options.MaxPositions || ledger.cash <= 0 {
		return
	}

	openSlots := ledger.options.MaxPositions - len(ledger.positions)
	if openSlots < 1 {
		return
	}

	notional := ledger.cash / float64(openSlots)
	fee := notional * feeRate
	if notional+fee > ledger.cash {
		notional = ledger.cash / (1 + feeRate)
		fee = notional * feeRate
	}
	if notional <= 0 || price <= 0 {
		return
	}

	ledger.cash -= notional + fee
	ledger.positions[symbol] = position{
		symbol: symbol,
		qty:    notional / price,
		entry:  price,
		mark:   price,
	}
	ledger.trades++
}

func (ledger *replayLedger) close(symbol string, price float64, feeRate float64) {
	pos, exists := ledger.positions[symbol]
	if !exists || price <= 0 {
		return
	}

	proceeds := pos.qty * price
	fee := proceeds * feeRate
	ledger.cash += proceeds - fee
	delete(ledger.positions, symbol)
	ledger.trades++
}

func (ledger *replayLedger) feeRate(actionType logic.ActionType) float64 {
	switch actionType {
	case logic.ActionLimit, logic.ActionIceberg, logic.ActionStopLossLimit,
		logic.ActionTakeProfitLimit, logic.ActionTrailingStopLimit:
		return ledger.options.MakerFeeRate
	default:
		return ledger.options.FeeRate
	}
}

func (ledger *replayLedger) Mark(prices map[string]float64) {
	for symbol, pos := range ledger.positions {
		if price := prices[symbol]; price > 0 {
			pos.mark = price
			ledger.positions[symbol] = pos
		}
	}

	wallet := ledger.wallet()
	if wallet > ledger.peakWallet {
		ledger.peakWallet = wallet
	}
	if ledger.peakWallet > 0 {
		drawdown := (ledger.peakWallet - wallet) / ledger.peakWallet
		if drawdown > ledger.maxDrawdown {
			ledger.maxDrawdown = drawdown
		}
	}
}

func (ledger *replayLedger) Result(frames []ReplayFrame) ReplayResult {
	ledger.Mark(nil)

	wallet := ledger.wallet()
	result := ReplayResult{
		Reward:      (wallet - ledger.start) / ledger.start,
		Wallet:      wallet,
		Cash:        ledger.cash,
		Trades:      ledger.trades,
		Positions:   len(ledger.positions),
		MaxDrawdown: ledger.maxDrawdown,
	}
	if len(frames) > 0 {
		result.StartedAt = frames[0].Time
		result.EndedAt = frames[len(frames)-1].Time
	}

	return result
}

func (ledger *replayLedger) wallet() float64 {
	total := ledger.cash
	for _, pos := range ledger.positions {
		price := pos.mark
		if price <= 0 {
			price = pos.entry
		}
		total += pos.qty * price * (1 - ledger.options.FeeRate)
	}

	return total
}

func (ledger *replayLedger) BalancesArtifact() *datura.Artifact {
	rows := []map[string]any{{"asset": "USD", "balance": ledger.cash}}
	for symbol, pos := range ledger.positions {
		base, _, _ := strings.Cut(symbol, "/")
		base = strings.ToUpper(strings.TrimSpace(base))
		if base == "" {
			continue
		}
		rows = append(rows, map[string]any{
			"asset":   base,
			"balance": pos.qty,
		})
	}

	return datura.Acquire("optimizer", datura.APPJSON).
		WithRole("balances").
		WithScope("balances").
		WithPayload(datura.Map[any]{
			"data": rows,
		}.Marshal())
}

func validateFrames(frames []ReplayFrame) error {
	if len(frames) == 0 {
		return fmt.Errorf("optimizer: no replay frames")
	}
	for index, frame := range frames {
		if frame.Time.IsZero() {
			return fmt.Errorf("optimizer: replay frame %d has zero time", index)
		}
		if len(frame.Artifacts) == 0 {
			return fmt.Errorf("optimizer: replay frame %d has no artifacts", index)
		}
	}

	return nil
}

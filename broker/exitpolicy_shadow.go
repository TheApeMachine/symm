package broker

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
)

type shadowJob struct {
	Symbol      string  `json:"symbol"`
	PolicyEpoch uint64  `json:"policy_epoch"`
	ExitTick    uint64  `json:"exit_tick"`
	TargetTick  uint64  `json:"target_tick"`
	Banked      float64 `json:"banked"`
	Qty         string  `json:"qty"`
	EntryPrice  string  `json:"entry_price"`
}

func policyShadowBars() int {
	bars := 32
	if system.Cfg != nil && system.Cfg.Learning != nil {
		if n := system.Cfg.Learning.PolicyShadowBars; n > 0 {
			bars = n
		} else if n := system.Cfg.Learning.MaxExcursionBars; n > 0 {
			bars = n * 4
		}
	}

	return bars
}

func (desk *Desk) schedulePostExitShadow(position *Position) {
	if desk == nil || position == nil || position.Symbol == "" {
		return
	}

	if position.ExitTick == 0 || position.Qty == nil || position.EntryPrice == nil {
		go desk.refreshPolicy(position.Symbol)
		return
	}

	banked := 0.0
	if position.PnLPct != nil {
		banked = position.PnLPct.Float64()
	}

	job := shadowJob{
		Symbol:      position.Symbol,
		PolicyEpoch: position.PolicyEpoch,
		ExitTick:    position.ExitTick,
		TargetTick:  position.ExitTick + uint64(policyShadowBars()),
		Banked:      banked,
		Qty:         position.Qty.String(),
		EntryPrice:  position.EntryPrice.String(),
	}

	if job.PolicyEpoch == 0 {
		job.PolicyEpoch = data.Epoch.Load()
	}

	desk.enqueueShadowJob(job)
	go desk.runShadowJob(job)
}

func (desk *Desk) enqueueShadowJob(job shadowJob) {
	if desk == nil || job.Symbol == "" {
		return
	}

	desk.shadowMu.Lock()
	desk.shadowJobs = append(desk.shadowJobs, job)
	desk.shadowMu.Unlock()
	desk.persistShadowQueue()
}

func (desk *Desk) dequeueShadowJob(job shadowJob) {
	if desk == nil {
		return
	}

	desk.shadowMu.Lock()
	filtered := desk.shadowJobs[:0]
	for _, existing := range desk.shadowJobs {
		if shadowJobEqual(existing, job) {
			continue
		}

		filtered = append(filtered, existing)
	}

	desk.shadowJobs = filtered
	desk.shadowMu.Unlock()
	desk.persistShadowQueue()
}

func shadowJobEqual(a, b shadowJob) bool {
	return a.Symbol == b.Symbol &&
		a.PolicyEpoch == b.PolicyEpoch &&
		a.ExitTick == b.ExitTick &&
		a.TargetTick == b.TargetTick &&
		a.Banked == b.Banked &&
		a.Qty == b.Qty &&
		a.EntryPrice == b.EntryPrice
}

func (desk *Desk) resumeShadowJobs() {
	if desk == nil {
		return
	}

	desk.shadowMu.Lock()
	jobs := append([]shadowJob(nil), desk.shadowJobs...)
	desk.shadowMu.Unlock()

	for _, job := range jobs {
		go desk.runShadowJob(job)
	}
}

func (desk *Desk) runShadowJob(job shadowJob) {
	if desk == nil {
		return
	}

	ctx := desk.Context()
	if ctx == nil {
		return
	}

	for {
		if ctx.Err() != nil {
			return
		}

		if data.Tick.Load() >= job.TargetTick {
			break
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Let the booter flush the last ticks in the shadow window.
	select {
	case <-ctx.Done():
		return
	case <-time.After(250 * time.Millisecond):
	}

	maxPct, bars := desk.measurePostExitRegret(job)
	if bars > 0 {
		desk.applyPostExitRegret(job.Symbol, job.Banked, maxPct, bars)
	}

	desk.dequeueShadowJob(job)
}

func (desk *Desk) measurePostExitRegret(job shadowJob) (maxPct float64, bars int) {
	if desk == nil || desk.history == nil || desk.history.store == nil || desk.price == nil {
		return 0, 0
	}

	qty, err := decimal.NewFromString(job.Qty)
	if err != nil || qty.Sign() <= 0 {
		return 0, 0
	}

	entry, err := decimal.NewFromString(job.EntryPrice)
	if err != nil || entry.Sign() <= 0 {
		return 0, 0
	}

	limit := policyShadowBars()
	prefix := fmt.Sprintf("workspace/tick/aggregate/%019d/", job.PolicyEpoch)
	ctx := desk.Context()

	maxPct = math.Inf(-1)

	for key := range desk.history.store.ListKeys(prefix) {
		tick, ok := tickFromAggregateKey(key)
		if !ok || tick <= job.ExitTick {
			continue
		}

		if bars >= limit {
			break
		}

		tickValue := desk.history.store.Download(ctx, key)
		if tickValue == nil || tickValue.Err != nil {
			continue
		}

		sellPrice := lastTradePrice(tickValue, job.Symbol)
		if sellPrice == nil {
			continue
		}

		pct := desk.price.PnLPctAtSellPrice(job.Symbol, qty, entry, nil, sellPrice)
		if pct == nil {
			continue
		}

		v := pct.Float64()
		if v > maxPct {
			maxPct = v
		}

		bars++
	}

	if bars == 0 || math.IsInf(maxPct, -1) {
		return 0, 0
	}

	return maxPct, bars
}

func (desk *Desk) applyPostExitRegret(symbol string, banked, postExitMax float64, shadowBars int) {
	known := desk.recordPolicy(symbol)
	if known == nil {
		return
	}

	regret := math.Max(0, postExitMax-banked)

	known.mu.Lock()

	for i := len(known.episodes) - 1; i >= 0; i-- {
		ep := &known.episodes[i]
		if ep.shadowBars > 0 {
			continue
		}

		if math.Abs(ep.banked-banked) > 1e-4 {
			continue
		}

		ep.postExitMaxPct = postExitMax
		ep.regretPct = regret
		ep.shadowBars = shadowBars
		break
	}

	known.rows = policyRows(known.episodes)
	known.mu.Unlock()

	desk.refreshPolicy(symbol)
}

func tickFromAggregateKey(key string) (uint64, bool) {
	key = strings.TrimPrefix(key, "/")
	parts := strings.Split(key, "/")
	if len(parts) < 2 {
		return 0, false
	}

	tickPart := parts[len(parts)-1]
	tick, err := strconv.ParseUint(tickPart, 10, 64)
	if err != nil {
		return 0, false
	}

	return tick, true
}

func lastTradePrice(tick *data.Value, symbol string) *decimal.Decimal {
	if tick == nil || symbol == "" {
		return nil
	}

	var trades []kraken.TradeData
	tick.Lookup(&trades, "spot:trade")

	if len(trades) == 0 {
		return nil
	}

	var price *decimal.Decimal
	for i := len(trades) - 1; i >= 0; i-- {
		if trades[i].Symbol != symbol {
			continue
		}

		p := trades[i].Price
		price = &p
		break
	}

	return price
}

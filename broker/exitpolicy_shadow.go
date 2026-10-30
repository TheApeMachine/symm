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

func (policy *ExitPolicy) SchedulePostExitShadow(position *Position) {
	if policy == nil || position == nil || position.Symbol == "" {
		return
	}

	if position.ExitTick == 0 || position.Qty == nil || position.EntryPrice == nil {
		go policy.Refresh(position.Symbol)
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

	policy.enqueueShadowJob(job)
	go policy.runShadowJob(job)
}

func (policy *ExitPolicy) enqueueShadowJob(job shadowJob) {
	if policy == nil || job.Symbol == "" {
		return
	}

	policy.shadowMu.Lock()
	policy.shadowJobs = append(policy.shadowJobs, job)
	policy.shadowMu.Unlock()
	policy.persistShadowQueue()
}

func (policy *ExitPolicy) dequeueShadowJob(job shadowJob) {
	if policy == nil {
		return
	}

	policy.shadowMu.Lock()
	filtered := policy.shadowJobs[:0]
	for _, existing := range policy.shadowJobs {
		if shadowJobEqual(existing, job) {
			continue
		}

		filtered = append(filtered, existing)
	}

	policy.shadowJobs = filtered
	policy.shadowMu.Unlock()
	policy.persistShadowQueue()
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

func (policy *ExitPolicy) ResumeShadowJobs() {
	if policy == nil {
		return
	}

	policy.shadowMu.Lock()
	jobs := append([]shadowJob(nil), policy.shadowJobs...)
	policy.shadowMu.Unlock()

	for _, job := range jobs {
		go policy.runShadowJob(job)
	}
}

func (policy *ExitPolicy) runShadowJob(job shadowJob) {
	if policy == nil || policy.desk == nil {
		return
	}

	ctx := policy.desk.Context()
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

	maxPct, bars := policy.measurePostExitRegret(job)
	if bars > 0 {
		policy.applyPostExitRegret(job.Symbol, job.Banked, maxPct, bars)
	}

	policy.dequeueShadowJob(job)
}

func (policy *ExitPolicy) measurePostExitRegret(job shadowJob) (maxPct float64, bars int) {
	desk := policy.desk
	if policy == nil || desk == nil || desk.history == nil || desk.history.store == nil || desk.price == nil {
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
	ctx := policy.desk.Context()

	maxPct = math.Inf(-1)

	for key := range policy.desk.history.store.ListKeys(prefix) {
		tick, ok := tickFromAggregateKey(key)
		if !ok || tick <= job.ExitTick {
			continue
		}

		if bars >= limit {
			break
		}

		tickValue := policy.desk.history.store.Download(ctx, key)
		if tickValue == nil || tickValue.Err != nil {
			continue
		}

		sellPrice := lastTradePrice(tickValue, job.Symbol)
		if sellPrice == nil {
			continue
		}

		pct := policy.desk.price.PnLPctAtSellPrice(job.Symbol, qty, entry, nil, sellPrice)
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

func (policy *ExitPolicy) applyPostExitRegret(symbol string, banked, postExitMax float64, shadowBars int) {
	known := policy.record(symbol)
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

	policy.Refresh(symbol)
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

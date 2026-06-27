package trader

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

// symState is one symbol's live quote for a single replay tick.
type symState struct {
	symbol string
	last   float64
	volume float64
	chgPct float64
	bid    float64
	ask    float64
}

// level is one (price, qty) order-book rung.
type level struct {
	price float64
	qty   float64
}

func tickerPayload(rows []symState, stamp time.Time) []byte {
	data := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, map[string]any{
			"symbol":     row.symbol,
			"bid":        row.bid,
			"bid_qty":    row.volume * 0.01,
			"ask":        row.ask,
			"ask_qty":    row.volume * 0.01,
			"last":       row.last,
			"volume":     row.volume,
			"vwap":       (row.bid + row.ask) / 2,
			"low":        row.last * 0.98,
			"high":       row.last * 1.02,
			"change":     row.last * row.chgPct / 100,
			"change_pct": row.chgPct,
			"timestamp":  stamp.Format(time.RFC3339Nano),
		})
	}
	out, _ := sonic.Marshal(map[string]any{"channel": "ticker", "type": "update", "data": data})
	return out
}

func tradePayload(symbol, side string, price, qty float64, stamp time.Time) []byte {
	out, _ := sonic.Marshal(map[string]any{
		"channel": "trade", "type": "update",
		"data": []map[string]any{{
			"symbol": symbol, "side": side, "price": price, "qty": qty,
			"ord_type": "market", "trade_id": 1, "timestamp": stamp.Format(time.RFC3339Nano),
		}},
	})
	return out
}

func bookPayload(symbol string, bids, asks []level, stamp time.Time) []byte {
	conv := func(levels []level) []map[string]any {
		out := make([]map[string]any, 0, len(levels))
		for _, lvl := range levels {
			out = append(out, map[string]any{"price": lvl.price, "qty": lvl.qty})
		}
		return out
	}
	out, _ := sonic.Marshal(map[string]any{
		"channel": "book", "type": "update",
		"data": []map[string]any{{
			"symbol": symbol, "bids": conv(bids), "asks": conv(asks),
			"timestamp": stamp.Format(time.RFC3339Nano),
		}},
	})
	return out
}

func insert(tree *dmt.Tree, role string, payload []byte, stamp int64) {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole(role)
	artifact.WithScope("update")
	artifact.WithPayload(payload)
	artifact.SetTimestamp(stamp)
	tree.Insert(artifact.Prefix("role", "timestamp"), artifact.Pack())
	artifact.Release()
}

// major is a top-of-book peer that drifts flat-to-down, like the BTC/ETH/SOL
// backdrop in the live screenshots (BTC -1.71%, ETH -1.53%).
type major struct {
	symbol string
	price  float64
	chgPct float64
	volume float64
}

func majorUniverse() []major {
	return []major{
		{"BTC/USD", 61630, -1.71, 4500},
		{"ETH/USD", 1645, -1.53, 60000},
		{"SOL/USD", 145, -1.20, 180000},
		{"XRP/USD", 0.52, -0.90, 9_000_000},
		{"ADA/USD", 0.38, -1.10, 12_000_000},
		{"DOT/USD", 4.10, -1.40, 800_000},
		{"LINK/USD", 11.20, -0.80, 600_000},
		{"ATOM/USD", 4.50, -1.00, 500_000},
		{"AVAX/USD", 21.00, -1.60, 400_000},
		{"MATIC/USD", 0.51, -1.30, 6_000_000},
	}
}

// slxProfile reproduces the SLX/USD session shape: baseline, leg-1 ignition,
// retrace, coil (volume builds, spread tightens), larger leg-2 ignition, then a
// grind. Returns price, executed volume, fractional spread, 24h change %, and a
// signed aggression in [-1,1] (buy pressure on legs, sell on retrace/grind).
func slxProfile(tick int) (last, vol, spread, chg, agg float64) {
	const base = 0.245
	switch {
	case tick < 25:
		last, vol, spread = base, 8000, 0.007
	case tick < 36:
		f := float64(tick-25) / 11
		last, vol, spread, agg = base+f*(0.305-base), 8000+f*82000, 0.007+f*0.003, 0.8
	case tick < 50:
		f := float64(tick-36) / 14
		last, vol, spread, agg = 0.305-f*(0.305-0.262), 90000-f*65000, 0.010-f*0.003, -0.5
	case tick < 74:
		f := float64(tick-50) / 24
		last, vol, spread, agg = 0.262+f*(0.272-0.262), 25000+f*40000, 0.007-f*0.0025, 0.2
	case tick < 100:
		f := float64(tick-74) / 26
		last, vol, spread, agg = 0.272+f*(0.379-0.272), 65000+f*135000, 0.0045+f*0.0045, 1.0
	default:
		f := math.Min(float64(tick-100)/20, 1)
		last, vol, spread, agg = 0.379-f*(0.379-0.372), 200000-f*150000, 0.009-f*0.003, -0.2
	}
	last *= 1 + 0.0015*math.Sin(float64(tick*3))
	chg = (last/base - 1) * 100
	return
}

// bdxnProfile reproduces the BDXN/USD thin-book trap: a long flat base on a 40%+
// spread and microscopic dollar volume, then a vertical that prints a huge %
// change on a hollow book — the move a principled system must REFUSE.
func bdxnProfile(tick int) (last, vol, spread, chg, agg float64) {
	const base = 0.00060
	switch {
	case tick < 70:
		last, vol, spread = base*(1+0.01*math.Sin(float64(tick))), 200000, 0.40
	case tick < 105:
		f := float64(tick-70) / 35
		last, vol, spread, agg = base+f*(0.00106-base), 200000+f*1_800_000, 0.43, 0.9
	default:
		last, vol, spread = 0.00106, 2_000_000, 0.43
	}
	chg = (last/base - 1) * 100
	return
}

func moverRow(symbol string, last, vol, spread, chg float64) symState {
	return symState{
		symbol: symbol, last: last, volume: vol, chgPct: chg,
		bid: last * (1 - spread/2), ask: last * (1 + spread/2),
	}
}

// moverBook builds a 3-deep ladder. On strong buy aggression a sell wall sits
// above the touch (the SRM/SLX resistance walls); bids stack underneath (coil).
func moverBook(row symState, vol, agg float64, stamp time.Time) []byte {
	bids := []level{
		{row.bid, vol * 0.02},
		{row.bid * 0.998, vol * 0.05},
		{row.bid * 0.995, vol * 0.10},
	}
	asks := []level{
		{row.ask, vol * 0.02},
		{row.ask * 1.002, vol * 0.05},
		{row.ask * 1.005, vol * 0.10},
	}
	if agg > 0.5 {
		asks[2] = level{row.last * 1.03, vol * 5} // sell wall at resistance
	}
	return bookPayload(row.symbol, bids, asks, stamp)
}

// catName resolves a measurement's winning category to its readable name.
func catName(m *datura.Artifact) string {
	idx := int(datura.Peek[float64](m, "output", "value"))
	if name, ok := logic.Categories[idx]; ok {
		return string(name)
	}
	return fmt.Sprintf("idx%d", idx)
}

func dumpMover(measurements []*datura.Artifact, symbol string) []string {
	var lines []string
	for _, m := range measurements {
		scope, _ := m.Scope()
		if scope != symbol {
			continue
		}
		origin, _ := m.Origin()
		conf := datura.Peek[float64](m, "output", "confidence")
		lines = append(lines, fmt.Sprintf("%s[c=%.2f %s]", origin, conf, catName(m)))
	}
	sort.Strings(lines)
	return lines
}

func filterScope(measurements []*datura.Artifact, symbol string) []*datura.Artifact {
	out := make([]*datura.Artifact, 0, len(measurements))
	for _, m := range measurements {
		if scope, _ := m.Scope(); scope == symbol {
			out = append(out, m)
		}
	}
	return out
}

func signalConfCat(measurements []*datura.Artifact, source logic.SourceType) (float64, string) {
	for _, m := range measurements {
		if origin, _ := m.Origin(); logic.SourceType(origin) == source {
			return datura.Peek[float64](m, "output", "confidence"), catName(m)
		}
	}
	return -1, "absent"
}

// entryBaselineReplica reproduces logic.confidenceBaseline(entry_baseline): the
// upper quartile (or median+MAD for small/tight samples) of all positive
// confidences this tick. This is the bar pumpdump confidence must clear to enter.
func entryBaselineReplica(measurements []*datura.Artifact) float64 {
	confs := make([]float64, 0, len(measurements))
	for _, m := range measurements {
		if c := datura.Peek[float64](m, "output", "confidence"); c > 0 {
			confs = append(confs, c)
		}
	}
	if len(confs) == 0 {
		return 0
	}
	sort.Float64s(confs)
	// upper quartile is the entry bar when the sample spans (nearest-rank).
	return confs[(3*len(confs))/4]
}

// saturationCount reports how many measurements report confidence pinned at ~1.0
// — the degeneracy that poisons the adaptive entry baseline.
func saturationCount(measurements []*datura.Artifact) (int, int) {
	total, sat := 0, 0
	for _, m := range measurements {
		c := datura.Peek[float64](m, "output", "confidence")
		if c <= 0 {
			continue
		}
		total++
		if c >= 0.999 {
			sat++
		}
	}
	return sat, total
}

// TestFunnelProbe drives a realistic sector-lift replay (a flat/down major-coin
// universe, a clean two-leg SLX ignition that SHOULD trade, and a hollow-book
// BDXN trap that SHOULD be refused) through the real funnel and reports, per
// tick, where the trade dies. Diagnostic — the truth is what the code does.
func TestFunnelProbe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 4, 8, nil)
	defer pool.Close()

	tree := dmt.NewTree("")
	crypto, err := NewCrypto(ctx, pool, tree)
	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}
	defer crypto.Close()

	// Strip Track-D field signals: manifold (GPU) and resonance are deferred
	// infrastructure, appear in NO entry branch, and only cost time + error spam.
	// The funnel never reaches the decider where they'd matter.
	delete(crypto.signals.signals, logic.SourceManifold)
	crypto.resonance = nil

	const numTicks = 120
	base := time.Now().UTC().Add(-time.Duration(numTicks+2) * time.Second).Truncate(time.Second)
	majors := majorUniverse()

	maxCandidates, maxChosen := 0, 0
	firstCandidateTick := -1

	for tick := 0; tick < numTicks; tick++ {
		stamp := base.Add(time.Duration(tick) * time.Second)
		ns := stamp.UnixNano()
		seq := ns

		rows := make([]symState, 0, len(majors)+2)
		for i, m := range majors {
			drift := m.chgPct / 100 / float64(numTicks) * float64(tick)
			wobble := 1 + 0.0008*math.Sin(float64(i*7+tick*3))
			price := m.price * (1 + drift) * wobble
			rows = append(rows, symState{
				symbol: m.symbol, last: price, volume: m.volume * wobble, chgPct: m.chgPct,
				bid: price * 0.9999, ask: price * 1.0001,
			})
		}

		slxLast, slxVol, slxSpread, slxChg, slxAgg := slxProfile(tick)
		bdxnLast, bdxnVol, bdxnSpread, bdxnChg, bdxnAgg := bdxnProfile(tick)
		slxRow := moverRow("SLX/USD", slxLast, slxVol, slxSpread, slxChg)
		bdxnRow := moverRow("BDXN/USD", bdxnLast, bdxnVol, bdxnSpread, bdxnChg)
		rows = append(rows, slxRow, bdxnRow)

		insert(tree, "ticker", tickerPayload(rows, stamp), seq)

		insert(tree, "book", moverBook(slxRow, slxVol, slxAgg, stamp), seq+1)
		insert(tree, "book", moverBook(bdxnRow, bdxnVol, bdxnAgg, stamp), seq+2)

		emitTrades := func(symbol string, last, vol, agg float64, startOffset int64) int64 {
			off := startOffset
			if agg > 0 {
				for n := 0; n < 3; n++ {
					insert(tree, "trade", tradePayload(symbol, "buy", last, vol*0.002*agg, stamp), seq+off)
					off++
				}
			} else if agg < 0 {
				for n := 0; n < 2; n++ {
					insert(tree, "trade", tradePayload(symbol, "sell", last, vol*0.002*(-agg), stamp), seq+off)
					off++
				}
			}
			return off
		}
		off := emitTrades("SLX/USD", slxLast, slxVol, slxAgg, 3)
		off = emitTrades("BDXN/USD", bdxnLast, bdxnVol, bdxnAgg, off)
		for i, m := range majors {
			side := "buy"
			if i%2 == 0 {
				side = "sell"
			}
			insert(tree, "trade", tradePayload(m.symbol, side, m.price, m.volume*0.0005, stamp), seq+off+int64(i))
		}

		crypto.signals.Observe(crypto.crossSection)
		measurements := uiMeasurements(crypto.signals.Measure(crypto.crossSection))

		balances := holdings(crypto.tree)
		actions := crypto.story.Update(measurements, balances)
		chosen, verdicts := crypto.decider.choose(measurements, actions, balances)

		if len(actions) > maxCandidates {
			maxCandidates = len(actions)
		}
		if len(chosen) > maxChosen {
			maxChosen = len(chosen)
		}
		if len(actions) > 0 && firstCandidateTick < 0 {
			firstCandidateTick = tick
		}

		report := tick == 24 || tick == 35 || tick == 49 || tick == 73 || tick == 99 || tick == 119 ||
			len(actions) > 0 || len(chosen) > 0
		if !report {
			continue
		}

		origins := map[string]struct{}{}
		for _, m := range measurements {
			origin, _ := m.Origin()
			origins[origin] = struct{}{}
		}

		t.Logf("tick %3d phase=%s | SLX last=%.4f chg=%+.1f%% | BDXN last=%.6f chg=%+.0f%% | meas=%d origins=%d cand=%d chosen=%d",
			tick, slxPhase(tick), slxLast, slxChg, bdxnLast, bdxnChg, len(measurements), len(origins), len(actions), len(chosen))
		if lines := dumpMover(measurements, "SLX/USD"); len(lines) > 0 {
			t.Logf("    SLX/USD : %v", lines)
		}

		// The exact entry gate for SLX: the ignition branch needs pumpdump
		// confidence >= entry_baseline AND the right category. Show all three.
		slxMeas := filterScope(measurements, "SLX/USD")
		sat, tot := saturationCount(slxMeas)
		bar := entryBaselineReplica(slxMeas)
		pdConf, pdCat := signalConfCat(slxMeas, logic.SourcePumpDump)
		t.Logf("    SLX GATE: entry_baseline(Q3)=%.3f vs pumpdump conf=%.3f cat=%s | saturated=%d/%d  -> clears=%t",
			bar, pdConf, pdCat, sat, tot, pdConf >= bar)
		for _, src := range []logic.SourceType{logic.SourceFluid, logic.SourceToxicity, logic.SourceDepthFlow, logic.SourceLiquidity} {
			c, cat := signalConfCat(slxMeas, src)
			t.Logf("      guard %-10s conf=%.2f cat=%s", src, c, cat)
		}
		for _, tr := range crypto.story.Traces() {
			if tr.Symbol != "SLX/USD" {
				continue
			}
			for _, st := range tr.Steps {
				t.Logf("      walk path=%v outcome=%s %s", st.Path, st.Outcome, st.Reason)
			}
		}
		if lines := dumpMover(measurements, "BDXN/USD"); len(lines) > 0 {
			t.Logf("    BDXN/USD: %v", lines)
		}
		for _, a := range actions {
			sym, _ := a.Scope()
			t.Logf("    candidate: sym=%s type=%s entry_conf=%.3f", sym,
				datura.Peek[string](a, "type"), datura.Peek[float64](a, "entry_confidence"))
		}
		for _, v := range verdicts {
			t.Logf("    verdict: sym=%s reason=%q score=%.4f", v.symbol, v.reason, v.score)
		}
	}

	t.Logf("=== SUMMARY: firstCandidateTick=%d maxCandidates=%d maxChosen=%d over %d ticks ===",
		firstCandidateTick, maxCandidates, maxChosen, numTicks)
	switch {
	case maxCandidates == 0:
		t.Logf("VERDICT: playbook proposed ZERO candidates across a realistic two-leg pump — funnel dies BEFORE the decider (signals or playbook)")
	case maxChosen == 0:
		t.Logf("VERDICT: playbook proposed candidates but decider chose ZERO — funnel dies AT the decider")
	default:
		t.Logf("VERDICT: system opened %d trade(s) — funnel reaches the desk", maxChosen)
	}
}

func slxPhase(tick int) string {
	switch {
	case tick < 25:
		return "base"
	case tick < 36:
		return "leg1"
	case tick < 50:
		return "retrace"
	case tick < 74:
		return "coil"
	case tick < 100:
		return "leg2"
	default:
		return "grind"
	}
}

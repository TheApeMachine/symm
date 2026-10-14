package strategy

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
inspectPosition adds the live state of the symbol's open positions to the
Training Measurement, so the positions dashboard reads them as plain Metrics.
Only values the desk and the book actually hold are written; an absent value
is left out rather than guessed.
*/
func (training *Training) inspectPosition(symbol string, metrics map[string]float64) {
	positions := training.desk.Open(symbol)

	if len(positions) == 0 {
		metrics["position_open"] = 0
		return
	}

	var qty, cost, fees float64
	var opened time.Time
	exiting := 0.0

	for _, position := range positions {
		if position.Closed() {
			continue
		}

		q := position.Qty()
		entry := position.EntryPrice()

		if q == nil {
			continue
		}

		qty += q.Float64()

		if entry != nil {
			cost += q.Float64() * entry.Float64()
		}

		if fee := position.EntryFee(); fee != nil {
			fees += fee.Float64()
		}

		if at := position.OpenedAt(); !at.IsZero() && (opened.IsZero() || at.Before(opened)) {
			opened = at
		}

		if position.Exiting() {
			exiting = 1
		}
	}

	if qty <= 0 {
		metrics["position_open"] = 0
		return
	}

	metrics["position_open"] = 1
	metrics["position_qty"] = qty
	metrics["position_exiting"] = exiting

	if !opened.IsZero() {
		metrics["position_held_s"] = time.Since(opened).Seconds()
	}

	if cost > 0 {
		metrics["position_entry"] = cost / qty
		metrics["position_cost"] = cost + fees
	}

	metrics["position_entry_fee"] = fees

	bid, ask := training.price.Touch(symbol)

	if bid != nil {
		metrics["position_bid"] = bid.Float64()
	}

	if ask != nil {
		metrics["position_ask"] = ask.Float64()
	}

	quantity := positions[0].Qty()

	if len(positions) > 1 || quantity == nil {
		return
	}

	surface, err := training.price.SellQuote(symbol, quantity)

	if err != nil || surface == nil || !surface.BookComplete {
		return
	}

	if surface.ExecutableQty != nil {
		// How many times over the visible bids could absorb the position.
		metrics["position_bid_depth_qty"] = surface.ExecutableQty.Float64()
		metrics["position_exit_room"] = surface.ExecutableQty.Float64() / qty
	}

	if surface.ExecutableVWAP != nil {
		metrics["position_exit_vwap"] = surface.ExecutableVWAP.Float64()
	}

	if surface.ExecutableValue != nil && cost > 0 {
		value := surface.ExecutableValue.Float64()
		metrics["position_value"] = value
		metrics["position_pnl"] = value - (cost + fees)
		metrics["position_pnl_pct"] = (value - (cost + fees)) / (cost + fees)
	}
}

/*
inspectMatch adds how the symbol's token path meets the stored paths: the
token sequence itself (region keys), how strong the current token is against
the tokenizer threshold, and for every prefix length how many stored paths
still match and which actions those agree on.
*/
func inspectMatch(current *policy, tokens []string, metrics map[string]float64) {
	metrics["token_depth"] = float64(len(tokens))
	metrics["stored_paths"] = float64(len(current.keys))
	metrics["stored_paths_hash"] = float64(keysHash(current.keys))

	for idx, token := range tokens {
		if idx >= data.TrainingPathSlots {
			break
		}

		for part, region := range strings.Split(token, "-") {
			key, err := strconv.Atoi(strings.TrimPrefix(region, "R"))

			if err != nil || key < 0 || key >= store.RegionCount || part >= data.RegionSlots {
				continue
			}

			metrics[fmt.Sprintf("path_token:%02d:%d", idx, part)] = float64(key)
		}
	}

	for depth := 1; depth <= min(len(tokens), data.TrainingPathSlots); depth++ {
		prefix := strings.Join(tokens[:depth], "/") + "/"
		start, _ := slices.BinarySearch(current.keys, prefix)
		var total, enter, exit, noop float64

		for _, key := range current.keys[start:] {
			if !strings.HasPrefix(key, prefix) {
				break
			}

			total++

			switch strings.TrimSuffix(key[len(prefix):][strings.LastIndex(key[len(prefix):], "/")+1:], ".bin") {
			case "enter":
				enter++
			case "exit":
				exit++
			case "noop":
				noop++
			}
		}

		metrics[fmt.Sprintf("match_candidates:%02d", depth)] = total
		metrics[fmt.Sprintf("match_enter:%02d", depth)] = enter
		metrics[fmt.Sprintf("match_exit:%02d", depth)] = exit
		metrics[fmt.Sprintf("match_noop:%02d", depth)] = noop
	}

	// The stored keys the whole live path is currently a prefix of, as
	// indices into the sorted key list /cognition/tree serves.
	if len(tokens) == 0 {
		return
	}

	prefix := strings.Join(tokens, "/") + "/"
	start, _ := slices.BinarySearch(current.keys, prefix)
	slot := 0

	for idx := start; idx < len(current.keys) && slot < data.MatchKeySlots; idx++ {
		if !strings.HasPrefix(current.keys[idx], prefix) {
			break
		}

		metrics[fmt.Sprintf("match_key:%03d", slot)] = float64(idx)
		slot++
	}
}

/*
keysHash is FNV-1a (32 bit) over the sorted keys joined by newlines, so the
dashboard can confirm its key list is the one the match_key indices refer to.
*/
func keysHash(keys []string) uint32 {
	hash := uint32(2166136261)

	for idx, key := range keys {
		if idx > 0 {
			hash = (hash ^ '\n') * 16777619
		}

		for i := 0; i < len(key); i++ {
			hash = (hash ^ uint32(key[i])) * 16777619
		}
	}

	return hash
}

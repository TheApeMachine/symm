package logic

import "strings"

/*
Balances holds portfolio inventory used during playbook evaluation.
*/
type Balances struct {
	Inventory map[string]float64 `json:"inventory"`
	Asset     []BalanceAsset     `json:"asset"`
}

/*
BalanceAsset is one asset row in a balances snapshot.
*/
type BalanceAsset struct {
	Asset   string  `json:"asset"`
	Balance float64 `json:"balance"`
}

/*
Held reports whether the ledger holds a positive balance for symbol, checking
both the inventory map and the asset rows.
*/
func (balances *Balances) Held(symbol string) bool {
	return symbolHeld(balances, symbol)
}

func symbolHeld(holdings *Balances, symbol string) bool {
	if holdings == nil || symbol == "" {
		return false
	}

	if quantity, ok := holdings.Inventory[symbol]; ok && quantity > 0 {
		return true
	}

	targets := assetCandidateSet(baseAsset(symbol))

	for asset, quantity := range holdings.Inventory {
		if quantity > 0 && assetMatchesAny(asset, targets) {
			return true
		}
	}

	for _, asset := range holdings.Asset {
		if asset.Balance > 0 && assetMatchesAny(asset.Asset, targets) {
			return true
		}
	}

	return false
}

func baseAsset(symbol string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(symbol))
	if base, _, ok := strings.Cut(trimmed, "/"); ok {
		return base
	}

	return trimmed
}

func assetCandidateSet(asset string) map[string]struct{} {
	candidates := make(map[string]struct{}, 4)

	addAssetCandidate(candidates, asset)
	addAssetCandidate(candidates, krakenBareAsset(asset))

	if _, ok := candidates["BTC"]; ok {
		addAssetCandidate(candidates, "XBT")
		addAssetCandidate(candidates, "XXBT")
	}
	if _, ok := candidates["XBT"]; ok {
		addAssetCandidate(candidates, "BTC")
		addAssetCandidate(candidates, "XXBT")
	}

	return candidates
}

func assetMatchesAny(asset string, candidates map[string]struct{}) bool {
	normalized := assetCandidateSet(asset)

	for candidate := range candidates {
		if _, ok := normalized[candidate]; ok {
			return true
		}
	}

	return false
}

func addAssetCandidate(candidates map[string]struct{}, asset string) {
	normalized := strings.ToUpper(strings.TrimSpace(asset))
	if normalized == "" {
		return
	}

	candidates[normalized] = struct{}{}
}

func krakenBareAsset(asset string) string {
	normalized := strings.ToUpper(strings.TrimSpace(asset))
	if len(normalized) == 4 && (normalized[0] == 'X' || normalized[0] == 'Z') {
		return normalized[1:]
	}

	return normalized
}

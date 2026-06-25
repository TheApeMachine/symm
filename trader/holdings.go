package trader

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
)

/*
holdings reads the most recent balances frame from the tree into a logic.Balances
the playbook and decider can evaluate against. Balances reach the tree through
the same frame.Publish path for both paper (kraken/paper) and the live private
websocket, so the trader queries one place regardless of mode — no per-tick REST
client, no relay layer (see AGENTS.md: traders query the tree).

It returns nil when no balances frame has been published yet, so callers treat
"no ledger" distinctly from "empty ledger" rather than inventing holdings.
*/
func holdings(tree *dmt.Tree) *logic.Balances {
	if tree == nil {
		return nil
	}

	var (
		latest      *datura.Artifact
		latestStamp int64
	)

	for artifact := range tree.Seek([]byte("balances/")) {
		if artifact.Timestamp() >= latestStamp {
			latest = artifact
			latestStamp = artifact.Timestamp()
		}
	}

	if latest == nil {
		return nil
	}

	balances := &logic.Balances{
		Inventory: make(map[string]float64),
		Asset:     make([]logic.BalanceAsset, 0),
	}

	// Balance frames wrap the asset rows under "asset" (kraken/frame WrapPayload).
	for rowIndex := 0; ; rowIndex++ {
		asset := datura.Peek[string](latest, "asset", rowIndex, "asset")

		if asset == "" {
			break
		}

		balance := datura.Peek[float64](latest, "asset", rowIndex, "balance")

		balances.Inventory[asset] = balance
		balances.Asset = append(balances.Asset, logic.BalanceAsset{
			Asset:   asset,
			Balance: balance,
		})
	}

	if len(balances.Asset) == 0 {
		return nil
	}

	return balances
}

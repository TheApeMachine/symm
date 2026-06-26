package market

import (
	"fmt"
	"sort"
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

/*
PositionReadings derives one reading per open (non-quote) position from the tree:
the balance frame gives quantity, the executions history gives the entry price of
the last buy/enter fill for that symbol, and the latest ticker frame gives the
mark. Stop state comes from broker stoploss artifacts. Unrealized P&L and percent
change are computed here so nothing downstream invents them. Returns nil when no
balances have been published yet — "no ledger" stays distinct from "flat".
*/
func PositionReadings(tree *dmt.Tree, quoteCurrency string) []map[string]any {
	if tree == nil {
		return nil
	}

	quote := strings.ToUpper(strings.TrimSpace(quoteCurrency))

	balances := latestBalances(tree)

	if balances == nil {
		return nil
	}

	positions := openPositions(balances, quote)
	if len(positions) == 0 {
		return []map[string]any{}
	}

	readings := make([]map[string]any, 0, len(positions))

	for _, position := range positions {
		mark := latestMark(tree, position.symbol)
		entry := latestEntryPrice(tree, position.symbol)

		// Without a recorded entry fill the mark is the only honest reference, so
		// P&L reads flat rather than inventing a gain against a guessed basis.
		if entry <= 0 {
			entry = mark
		}

		reading := map[string]any{
			"symbol":    position.symbol,
			"asset":     position.asset,
			"quote":     quote,
			"quantity":  position.quantity,
			"entry":     entry,
			"mark":      mark,
			"value":     mark * position.quantity,
			"updatedAt": balances.Timestamp(),
		}

		if mark > 0 && entry > 0 {
			reading["unrealizedPnl"] = (mark - entry) * position.quantity
			reading["changePct"] = (mark - entry) / entry * 100
		}

		if stop := latestStop(tree, position.symbol); stop != nil {
			reading["stop"] = datura.Peek[float64](stop, "stop")
			reading["peak"] = datura.Peek[float64](stop, "peak")
			reading["offset"] = datura.Peek[float64](stop, "offset")
			reading["stopSide"] = datura.Peek[string](stop, "side")
		}

		readings = append(readings, reading)
	}

	sort.Slice(readings, func(first, second int) bool {
		return strings.Compare(
			fmt.Sprint(readings[first]["symbol"]),
			fmt.Sprint(readings[second]["symbol"]),
		) < 0
	})

	return readings
}

type openPosition struct {
	asset    string
	symbol   string
	quantity float64
}

func openPositions(balances *datura.Artifact, quote string) []openPosition {
	positions := make([]openPosition, 0)

	for rowIndex := 0; ; rowIndex++ {
		asset := datura.Peek[string](balances, "asset", rowIndex, "asset")

		if asset == "" {
			break
		}

		quantity := datura.Peek[float64](balances, "asset", rowIndex, "balance")

		// The quote currency funds entries rather than being a position, and a
		// dust balance is not an open position.
		if strings.ToUpper(asset) == quote || quantity <= 0 {
			continue
		}

		positions = append(positions, openPosition{
			asset:    asset,
			symbol:   asset + "/" + quote,
			quantity: quantity,
		})
	}

	return positions
}

/*
latestBalances returns the most recent balances frame in the tree, or nil when
none has been published.
*/
func latestBalances(tree *dmt.Tree) *datura.Artifact {
	var (
		latest *datura.Artifact
		stamp  int64
	)

	for artifact := range tree.Seek([]byte("balances/")) {
		if artifact.Timestamp() >= stamp {
			latest = artifact
			stamp = artifact.Timestamp()
		}
	}

	return latest
}

/*
latestEntryPrice walks executions for one held symbol and returns the freshest
opening (buy/enter) fill. Sells are ignored — they close a position, they do not
set an entry basis.
*/
func latestEntryPrice(tree *dmt.Tree, symbol string) float64 {
	type stampedEntry struct {
		price float64
		stamp int64
	}

	var latest stampedEntry
	target := strings.ToUpper(symbol)

	for artifact := range tree.Seek([]byte("executions/")) {
		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")

			if symbol == "" {
				break
			}

			side := strings.ToLower(datura.Peek[string](artifact, "data", rowIndex, "side"))

			if side != "buy" && side != "enter" {
				continue
			}

			price := datura.Peek[float64](artifact, "data", rowIndex, "avg_price")

			if price <= 0 {
				price = datura.Peek[float64](artifact, "data", rowIndex, "last_price")
			}

			if price <= 0 {
				price = datura.Peek[float64](artifact, "data", rowIndex, "price")
			}

			if price <= 0 {
				continue
			}

			if strings.ToUpper(symbol) != target {
				continue
			}

			stamp := artifact.Timestamp()

			if latest.stamp > stamp {
				continue
			}

			latest = stampedEntry{price: price, stamp: stamp}
		}
	}

	return latest.price
}

/*
latestMark returns the freshest traded price for one held symbol.
*/
func latestMark(tree *dmt.Tree, symbol string) float64 {
	type stampedMark struct {
		price float64
		stamp int64
	}

	var latest stampedMark
	target := strings.ToUpper(symbol)

	for artifact := range tree.Seek([]byte("ticker/")) {
		for rowIndex := 0; ; rowIndex++ {
			row, err := SymbolFromTicker(artifact, rowIndex)

			if err != nil || row == nil {
				break
			}

			if strings.ToUpper(row.Name) != target {
				continue
			}

			stamp := artifact.Timestamp()

			if latest.stamp > stamp {
				continue
			}

			latest = stampedMark{price: row.Price, stamp: stamp}
		}
	}

	return latest.price
}

func latestStop(tree *dmt.Tree, symbol string) *datura.Artifact {
	var latest *datura.Artifact

	for artifact := range tree.Seek([]byte("stoploss/" + symbol)) {
		if latest != nil && latest.Timestamp() > artifact.Timestamp() {
			continue
		}

		latest = artifact
	}

	return latest
}

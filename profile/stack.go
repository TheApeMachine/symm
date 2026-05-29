package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/causal"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/correlation"
	"github.com/theapemachine/symm/cvd"
	"github.com/theapemachine/symm/depthflow"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/exhaust"
	"github.com/theapemachine/symm/fluid"
	"github.com/theapemachine/symm/hawkes"
	"github.com/theapemachine/symm/kraken/client"
	"github.com/theapemachine/symm/kraken/core"
	"github.com/theapemachine/symm/leadlag"
	"github.com/theapemachine/symm/liquidity"
	"github.com/theapemachine/symm/price"
	"github.com/theapemachine/symm/pumpdump"
	"github.com/theapemachine/symm/sentiment"
	"github.com/theapemachine/symm/toxicity"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/wallet"
)

/*
Stack wires the same systems as cmd/root.go for integration profiling.
*/
type Stack struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	PublicClient *client.PublicClient
	systems      []engine.System
	tickWait     sync.WaitGroup
}

func NewStack(ctx context.Context, pool *qpool.Q) (*Stack, error) {
	ctx, cancel := context.WithCancel(ctx)

	predictions := price.NewPrediction(ctx, pool)
	publicClient := client.NewPublicClient(ctx, pool, core.KRAKEN_WS_URL)

	stack := &Stack{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		PublicClient: publicClient,
		systems: []engine.System{
			publicClient,
			pumpdump.NewPumpDump(ctx, pool),
			correlation.NewSignal(ctx, pool),
			depthflow.NewDepthFlow(ctx, pool),
			hawkes.NewHawkes(ctx, pool),
			leadlag.NewLeadLag(ctx, pool),
			liquidity.NewLiquidity(ctx, pool),
			sentiment.NewSentiment(ctx, pool),
			fluid.NewFluid(ctx, pool),
			causal.NewCausal(ctx, pool),
			cvd.NewCVD(ctx, pool),
			toxicity.NewToxicity(ctx, pool),
			exhaust.NewExhaust(ctx, pool),
			predictions,
			trader.NewCrypto(
				ctx,
				pool,
				wallet.NewWallet(
					wallet.PaperWallet,
					config.System.QuoteCurrency,
					config.System.WalletEUR,
					config.System.TakerFeePct,
				),
				predictions,
			),
		},
	}

	for _, system := range stack.systems {
		if err := system.Start(); err != nil {
			cancel()
			return nil, err
		}
	}

	return stack, nil
}

func (stack *Stack) StartTicks() {
	for _, system := range stack.systems {
		stack.tickWait.Go(func() {
			_ = system.Tick()
		})
	}
}

func (stack *Stack) Close() {
	stack.cancel()

	for _, system := range stack.systems {
		_ = system.Close()
	}

	stack.tickWait.Wait()
}

func LoadLines(symbolCount int, rounds int) ([][]byte, error) {
	if symbolCount < 1 {
		symbolCount = 16
	}

	if rounds < 1 {
		rounds = 32
	}

	symbols := make([]string, symbolCount)

	for index := 0; index < symbolCount; index++ {
		symbols[index] = fmt.Sprintf("SYM%d/EUR", index)
	}

	pairs := make([]map[string]any, 0, symbolCount)

	for index, symbol := range symbols {
		pairs = append(pairs, map[string]any{
			"symbol":          symbol,
			"base":            fmt.Sprintf("SYM%d", index),
			"quote":           "EUR",
			"status":          "online",
			"qty_precision":   8,
			"qty_increment":   1e-8,
			"price_precision": 2,
			"cost_precision":  5,
			"marginable":      true,
			"has_index":       true,
			"cost_min":        0.45,
			"price_increment": 0.01,
			"qty_min":         0.0001,
		})
	}

	catalog, err := json.Marshal(map[string]any{
		"channel": "instrument",
		"type":    "snapshot",
		"data": map[string]any{
			"assets": []any{},
			"pairs":  pairs,
		},
	})

	if err != nil {
		return nil, err
	}

	lines := make([][]byte, 0, 1+rounds*symbolCount*3)
	lines = append(lines, catalog)

	for round := 0; round < rounds; round++ {
		for index, symbol := range symbols {
			priceLevel := 100 + float64(round)*0.01 + float64(index)*0.1

			ticker, err := json.Marshal(map[string]any{
				"channel": "ticker",
				"type":    "update",
				"data": []map[string]any{{
					"symbol":     symbol,
					"last":       priceLevel,
					"bid":        priceLevel - 0.05,
					"ask":        priceLevel + 0.05,
					"volume":     1000 + float64(round),
					"change_pct": float64(round%5) * 0.1,
				}},
			})

			if err != nil {
				return nil, err
			}

			book, err := json.Marshal(map[string]any{
				"channel": "book",
				"type":    "update",
				"data": []map[string]any{{
					"symbol": symbol,
					"bids": []map[string]any{
						{"price": priceLevel - 0.05, "qty": 1.2 + float64(round%3)*0.1},
						{"price": priceLevel - 0.10, "qty": 2.4},
					},
					"asks": []map[string]any{
						{"price": priceLevel + 0.05, "qty": 0.8 + float64(round%2)*0.2},
						{"price": priceLevel + 0.10, "qty": 1.6},
					},
				}},
			})

			if err != nil {
				return nil, err
			}

			trade, err := json.Marshal(map[string]any{
				"channel": "trade",
				"type":    "update",
				"data": []map[string]any{{
					"symbol":    symbol,
					"side":      "buy",
					"price":     priceLevel,
					"qty":       0.05 + float64(round%4)*0.01,
					"timestamp": fmt.Sprintf("2026-05-29T12:%02d:%02d.000000Z", round%60, index%60),
				}},
			})

			if err != nil {
				return nil, err
			}

			lines = append(lines, ticker, book, trade)
		}
	}

	return lines, nil
}

func DefaultWorkerCount() int {
	return runtime.NumCPU() * 4
}

package metrics

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

/*
Trade is one closed round trip used by offline validation.
*/
type Trade struct {
	ID            string
	Symbol        string
	EntryAt       time.Time
	ExitAt        time.Time
	EntryNotional float64
	ExitNotional  float64
	Fees          float64
}

/*
Summary contains deterministic strategy performance statistics.
*/
type Summary struct {
	Trades              int
	Wins                int
	Losses              int
	NetPnL              float64
	ReturnFraction      float64
	MaxDrawdownFraction float64
	Turnover            float64
	HitRate             float64
	AverageWin          float64
	AverageLoss         float64
	TimeInMarket        time.Duration
}

/*
PerformanceCalculator computes validation metrics from closed trades.
*/
type PerformanceCalculator struct {
	startingCapital float64
}

func NewPerformanceCalculator(startingCapital float64) (*PerformanceCalculator, error) {
	if startingCapital <= 0 {
		return nil, errors.New("research metrics: starting capital must be positive")
	}

	return &PerformanceCalculator{startingCapital: startingCapital}, nil
}

func (calculator *PerformanceCalculator) Summarize(trades []Trade) (Summary, error) {
	if calculator == nil || calculator.startingCapital <= 0 {
		return Summary{}, errors.New("research metrics: calculator is not configured")
	}

	orderedTrades, err := validatedTrades(trades)

	if err != nil {
		return Summary{}, err
	}

	summary := Summary{
		Trades: len(orderedTrades),
	}

	if len(orderedTrades) == 0 {
		return summary, nil
	}

	equity := calculator.startingCapital
	peakEquity := calculator.startingCapital
	winPnL := 0.0
	lossPnL := 0.0
	intervals := make([]tradeInterval, 0, len(orderedTrades))

	for _, trade := range orderedTrades {
		netPnL := trade.ExitNotional - trade.EntryNotional - trade.Fees
		summary.NetPnL += netPnL
		summary.Turnover += trade.EntryNotional / calculator.startingCapital
		equity += netPnL

		if equity > peakEquity {
			peakEquity = equity
		}

		if peakEquity > 0 {
			drawdown := (peakEquity - equity) / peakEquity

			if drawdown > summary.MaxDrawdownFraction {
				summary.MaxDrawdownFraction = drawdown
			}
		}

		if netPnL > 0 {
			summary.Wins++
			winPnL += netPnL
		}

		if netPnL < 0 {
			summary.Losses++
			lossPnL += netPnL
		}

		intervals = append(intervals, tradeInterval{
			start: trade.EntryAt,
			end:   trade.ExitAt,
		})
	}

	summary.ReturnFraction = summary.NetPnL / calculator.startingCapital
	summary.HitRate = hitRate(summary.Wins, summary.Trades)
	summary.AverageWin = averagePnL(winPnL, summary.Wins)
	summary.AverageLoss = averagePnL(lossPnL, summary.Losses)
	summary.TimeInMarket = mergedDuration(intervals)

	return summary, nil
}

func validatedTrades(trades []Trade) ([]Trade, error) {
	orderedTrades := append([]Trade(nil), trades...)

	for _, trade := range orderedTrades {
		if trade.Symbol == "" {
			return nil, errors.New("research metrics: trade symbol is required")
		}

		if trade.EntryAt.IsZero() || trade.ExitAt.IsZero() {
			return nil, fmt.Errorf("research metrics: trade %q timestamps are required", trade.ID)
		}

		if trade.ExitAt.Before(trade.EntryAt) {
			return nil, fmt.Errorf("research metrics: trade %q exits before entry", trade.ID)
		}

		if trade.EntryNotional < 0 || trade.ExitNotional < 0 || trade.Fees < 0 {
			return nil, fmt.Errorf("research metrics: trade %q economics must be non-negative", trade.ID)
		}
	}

	sort.Slice(orderedTrades, func(leftIndex, rightIndex int) bool {
		return orderedTrades[leftIndex].ExitAt.Before(orderedTrades[rightIndex].ExitAt)
	})

	return orderedTrades, nil
}

func hitRate(wins int, trades int) float64 {
	if trades == 0 {
		return 0
	}

	return float64(wins) / float64(trades)
}

func averagePnL(total float64, count int) float64 {
	if count == 0 {
		return 0
	}

	return total / float64(count)
}

type tradeInterval struct {
	start time.Time
	end   time.Time
}

func mergedDuration(intervals []tradeInterval) time.Duration {
	if len(intervals) == 0 {
		return 0
	}

	sort.Slice(intervals, func(leftIndex, rightIndex int) bool {
		return intervals[leftIndex].start.Before(intervals[rightIndex].start)
	})

	total := time.Duration(0)
	current := intervals[0]

	for _, interval := range intervals[1:] {
		if interval.start.After(current.end) {
			total += current.end.Sub(current.start)
			current = interval
			continue
		}

		if interval.end.After(current.end) {
			current.end = interval.end
		}
	}

	total += current.end.Sub(current.start)

	return total
}

package broker

import (
	"errors"
	"fmt"
	"time"

	"github.com/theapemachine/symm/logic"
)

var (
	ErrQuoteStale      = errors.New("broker: quote stale")
	ErrSpreadTooWide   = errors.New("broker: spread too wide")
	ErrSlippageTooHigh = errors.New("broker: estimated slippage too high")
)

/*
QuoteSnapshot is the desk's view of executable quote quality for one symbol.
*/
type QuoteSnapshot struct {
	Symbol    string
	Mark      float64
	Bid       float64
	Ask       float64
	UpdatedAt time.Time
}

/*
PreTradeGate enforces configured quote-quality limits before entries.
*/
type PreTradeGate struct{}

func (gate *PreTradeGate) CheckEntry(
	action *logic.Action,
	risk RiskContext,
	quote QuoteSnapshot,
) error {
	if action == nil {
		return fmt.Errorf("broker: nil action")
	}

	if action.Type.IsExit() {
		return nil
	}

	if risk.MaxQuoteAge > 0 {
		if quote.UpdatedAt.IsZero() {
			return ErrQuoteStale
		}

		if time.Since(quote.UpdatedAt) > risk.MaxQuoteAge {
			return ErrQuoteStale
		}
	}

	spreadBps := spreadBps(quote)

	if risk.MaxSpreadBps > 0 && spreadBps > risk.MaxSpreadBps {
		return ErrSpreadTooWide
	}

	if risk.MaxSlippageBps > 0 {
		estimatedSlippageBps := spreadBps / 2

		if estimatedSlippageBps > risk.MaxSlippageBps {
			return ErrSlippageTooHigh
		}
	}

	return nil
}

func spreadBps(quote QuoteSnapshot) float64 {
	if quote.Bid <= 0 || quote.Ask <= 0 || quote.Ask < quote.Bid {
		return 0
	}

	mid := (quote.Bid + quote.Ask) / 2

	if mid <= 0 {
		return 0
	}

	return (quote.Ask - quote.Bid) / mid * 10_000
}

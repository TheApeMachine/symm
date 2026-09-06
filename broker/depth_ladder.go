package broker

import "math/big"

/*
DepthLadder is the displayed depth a decision was sized against, one entry per
price level in sweep order. It is retained so a later fill can be limited to
liquidity that was actually there when the decision was made and is still there
when it executes.
*/
type DepthLadder struct {
	prices     [ladderLevels]big.Rat
	quantities [ladderLevels]big.Rat
	Count      int
}

/*
ladderLevels is the existing retained decision-depth budget. Levels beyond it
are not eligible for a later constrained fill because they were not retained.
*/
const ladderLevels = 16

/* Record captures one displayed level in sweep order. */
func (ladder *DepthLadder) Record(price, quantity *big.Rat) {
	if ladder == nil || ladder.Count >= ladderLevels {
		return
	}

	ladder.prices[ladder.Count].Set(price)
	ladder.quantities[ladder.Count].Set(quantity)
	ladder.Count++
}

/*
Surviving returns how much of a level's displayed quantity may be taken, given
what stood at that price when the decision was made. Liquidity that appeared
after the decision was never available to it, and liquidity that has since been
cancelled is gone: an unlimited sweep of the current book models neither, and
credits the account with fills it would have raced and lost.
*/
func (ladder *DepthLadder) Surviving(price, quantity *big.Rat) *big.Rat {
	if ladder == nil || ladder.Count == 0 {
		return quantity
	}

	for index := range ladder.Count {
		if ladder.prices[index].Cmp(price) != 0 {
			continue
		}

		if ladder.quantities[index].Cmp(quantity) < 0 {
			return &ladder.quantities[index]
		}

		return quantity
	}

	// This price was not displayed when the decision was sized, so none of it
	// was available to that decision.
	return new(big.Rat)
}

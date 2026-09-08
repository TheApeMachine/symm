package strategy

import (
	"maps"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/hindsight"
)

/* portfolioPosition retains one lot, its last executable mark and its next local reduction. */
type portfolioPosition struct {
	wallet    virtualWallet
	value     *decimal.Decimal
	complete  bool
	pending   LearningAction
	requested *decimal.Decimal
}

/* VirtualPortfolio is one finite shared wallet; its symbols never own separate cash. */
type VirtualPortfolio struct {
	cash       *decimal.Decimal
	positions  map[string]*portfolioPosition
	initial    *decimal.Decimal
	pending    *EntryCandidate
	receipt    *AllocationReceipt
	version    uint64
	marked     *decimal.Decimal
	scratch    *decimal.Decimal
	incomplete int
	inventory  map[string]string
}

/* NewVirtualPortfolio establishes a single account's known capital. */
func NewVirtualPortfolio(initial *decimal.Decimal) *VirtualPortfolio {
	portfolio := &VirtualPortfolio{initial: initial.Copy(), positions: make(map[string]*portfolioPosition), inventory: make(map[string]string)}
	portfolio.cash, portfolio.marked = initial.SetScale(decimal.DefaultScale), zero
	return portfolio
}

/* Allocate commits an executable candidate only once against this account's finite cash. */
func (portfolio *VirtualPortfolio) Allocate(candidate *EntryCandidate, receipt *AllocationReceipt) bool {
	if portfolio.pending != nil || candidate.cost.Cmp(portfolio.cash) > 0 {
		return false
	}
	portfolio.pending, portfolio.receipt = candidate, receipt
	return true
}

/*
Step marks only the changed symbol and applies orders to later surviving spot depth.
Local exit decisions use this portfolio's own inventory. Local isolated wallets
and their evidence are untouched by these selected-position outcomes.
*/
func (portfolio *VirtualPortfolio) Step(local *LocalLearning, market *learningMarket, book *spotbook.Book) error {
	position := portfolio.positions[market.symbol]

	if candidate := portfolio.pending; candidate != nil && candidate.Record.Symbol == market.symbol {
		if position == nil {
			position = &portfolioPosition{value: zero}
			portfolio.incomplete++
			if err := position.wallet.initialize(portfolio.initial, local.price, market.symbol); err != nil {
				return err
			}
			portfolio.positions[market.symbol] = position
		}
		position.wallet.cash = portfolio.cash
		portfolio.scratch = position.wallet.quantity
		if _, _, _, err := position.wallet.fill(book, candidate.action, candidate.quantity); err != nil {
			return err
		}
		result := hindsight.AllocationResult{State: "aborted", At: market.at, Detail: "no surviving executable depth filled the virtual allocation"}

		if position.wallet.quantity.Cmp(portfolio.scratch) > 0 {
			result.State, result.Detail = "filled", ""
		}
		portfolio.receipt.Report(result)
		portfolio.cash = position.wallet.cash
		position.wallet.cash = zero
		portfolio.pending, portfolio.receipt = nil, nil
		portfolio.inventory = maps.Clone(portfolio.inventory)
		portfolio.inventory[market.symbol] = position.wallet.quantity.String()
	}

	if position == nil {
		return nil
	}

	if position.requested != nil {
		if _, _, _, err := position.wallet.fill(book, position.pending, position.requested); err != nil {
			return err
		}
		portfolio.cash = portfolio.cash.Add(position.wallet.cash)
		position.wallet.cash = zero
		position.requested = nil
		portfolio.inventory = maps.Clone(portfolio.inventory)
		portfolio.inventory[market.symbol] = position.wallet.quantity.String()
	}

	if position.wallet.quantity.Sign() == 0 {
		portfolio.marked = portfolio.marked.Sub(position.value)

		if !position.complete {
			portfolio.incomplete--
		}
		portfolio.inventory = maps.Clone(portfolio.inventory)
		delete(portfolio.inventory, market.symbol)
		delete(portfolio.positions, market.symbol)
		return nil
	}
	mark, complete, err := position.wallet.mark(book)

	if err != nil {
		return err
	}

	if complete != position.complete {
		if complete {
			portfolio.incomplete--
		}

		if !complete {
			portfolio.incomplete++
		}
	}
	position.complete = complete

	if !complete {
		return nil
	}
	portfolio.marked = portfolio.marked.Sub(position.value)
	position.value = mark
	portfolio.marked = portfolio.marked.Add(position.value)
	context := append([]uint64(nil), market.PrecursorContext()...)
	actions, err := position.wallet.actions(book, nil)

	if err != nil {
		return err
	}
	action, _, err := local.Knowledge.Select(market.symbol, position.wallet.state(), context, actions, false)

	if err != nil {
		return err
	}

	if action.Reduce {
		position.pending = action
		position.requested, err = position.wallet.request(book, action, 1)

		if err != nil {
			return err
		}
	}
	return nil
}

/* Snapshot returns one finite account mark using each position's latest observable depth. */
func (portfolio *VirtualPortfolio) Snapshot(at time.Time) AccountState {
	portfolio.version++
	portfolio.scratch = portfolio.cash.Add(portfolio.marked)
	state := AccountState{Committed: "0", Cash: portfolio.cash.String(), ActualCash: portfolio.cash.String(), Positions: portfolio.inventory,
		Mark: EquityMark{At: at, Version: portfolio.version, HasFunding: true}, Complete: portfolio.incomplete == 0}
	state.Mark.Equity = portfolio.scratch.Float64()

	if portfolio.pending != nil {
		state.Committed = portfolio.pending.cost.String()
		portfolio.scratch = portfolio.cash.Sub(portfolio.pending.cost)
		state.Cash = portfolio.scratch.String()
	}

	return state
}

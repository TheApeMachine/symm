package strategy

import (
	"maps"
	"math/big"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/hindsight"
)

/* portfolioPosition retains one lot, its last executable mark and its next local reduction. */
type portfolioPosition struct {
	wallet    virtualWallet
	value     big.Rat
	complete  bool
	pending   LearningAction
	requested *big.Rat
	ladder    depthLadder
}

/* VirtualPortfolio is one finite shared wallet; its symbols never own separate cash. */
type VirtualPortfolio struct {
	cash       big.Rat
	positions  map[string]*portfolioPosition
	initial    *decimal.Decimal
	pending    *EntryCandidate
	receipt    *AllocationReceipt
	version    uint64
	marked     big.Rat
	scratch    big.Rat
	incomplete int
	inventory  map[string]string
}

/* NewVirtualPortfolio establishes a single account's known capital. */
func NewVirtualPortfolio(initial *decimal.Decimal) *VirtualPortfolio {
	portfolio := &VirtualPortfolio{initial: initial.Copy(), positions: make(map[string]*portfolioPosition), inventory: make(map[string]string)}
	portfolio.cash.Set(initial.Rat())
	return portfolio
}

/* Allocate commits an executable candidate only once against this account's finite cash. */
func (portfolio *VirtualPortfolio) Allocate(candidate *EntryCandidate, receipt *AllocationReceipt) bool {
	if portfolio.pending != nil || candidate.cost.Cmp(&portfolio.cash) > 0 {
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
			position = &portfolioPosition{}
			portfolio.incomplete++
			position.wallet.initialize(portfolio.initial, local.pair(market.symbol), local.fee(market.symbol).Fee)
			portfolio.positions[market.symbol] = position
		}
		position.wallet.cash.Set(&portfolio.cash)
		portfolio.scratch.Set(&position.wallet.quantity)
		position.wallet.fill(book, candidate.action, candidate.quantity, &candidate.ladder)
		result := hindsight.AllocationResult{State: "aborted", At: market.at, Detail: "no surviving executable depth filled the virtual allocation"}

		if position.wallet.quantity.Cmp(&portfolio.scratch) > 0 {
			result.State, result.Detail = "filled", ""
		}
		portfolio.receipt.Report(result)
		portfolio.cash.Set(&position.wallet.cash)
		position.wallet.cash.SetInt64(0)
		portfolio.pending, portfolio.receipt = nil, nil
		portfolio.inventory = maps.Clone(portfolio.inventory)
		portfolio.inventory[market.symbol] = position.wallet.quantity.RatString()
	}

	if position == nil {
		return nil
	}

	if position.requested != nil {
		position.wallet.fill(book, position.pending, position.requested, &position.ladder)
		portfolio.cash.Add(&portfolio.cash, &position.wallet.cash)
		position.wallet.cash.SetInt64(0)
		position.requested = nil
		portfolio.inventory = maps.Clone(portfolio.inventory)
		portfolio.inventory[market.symbol] = position.wallet.quantity.RatString()
	}

	if position.wallet.quantity.Sign() == 0 {
		portfolio.marked.Sub(&portfolio.marked, &position.value)

		if !position.complete {
			portfolio.incomplete--
		}
		portfolio.inventory = maps.Clone(portfolio.inventory)
		delete(portfolio.inventory, market.symbol)
		delete(portfolio.positions, market.symbol)
		return nil
	}
	mark, complete := position.wallet.mark(book)

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
	portfolio.marked.Sub(&portfolio.marked, &position.value)
	position.value.Set(mark)
	portfolio.marked.Add(&portfolio.marked, &position.value)
	state := portfolio.Snapshot(market.at)
	context := position.wallet.context(market.sequence, book, state.Mark.Equity, nil)
	actions := position.wallet.actions(book, nil)
	action, _, err := local.Knowledge.Select(market.symbol, context, actions, false)

	if err != nil {
		return err
	}

	if action.Reduce {
		position.pending = action
		position.requested = position.wallet.request(book, action, 1, &position.ladder)
	}
	return nil
}

/* Snapshot returns one finite account mark using each position's latest observable depth. */
func (portfolio *VirtualPortfolio) Snapshot(at time.Time) AccountState {
	portfolio.version++
	portfolio.scratch.Add(&portfolio.cash, &portfolio.marked)
	state := AccountState{Committed: "0", Cash: portfolio.cash.RatString(), ActualCash: portfolio.cash.RatString(), Positions: portfolio.inventory,
		Mark: EquityMark{At: at, Version: portfolio.version, HasFunding: true}, Complete: portfolio.incomplete == 0}
	state.Mark.Equity, _ = portfolio.scratch.Float64()

	if portfolio.pending != nil {
		state.Committed = portfolio.pending.cost.RatString()
		portfolio.scratch.Sub(&portfolio.cash, portfolio.pending.cost)
		state.Cash = portfolio.scratch.RatString()
	}

	return state
}

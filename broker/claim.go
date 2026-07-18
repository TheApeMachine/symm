package broker

import "github.com/krakenfx/api-go/v2/pkg/decimal"

/*
Claim owns one Book reservation id on a Position so cancel and fail paths
release without guessing amounts from Decisions.
*/
type Claim struct {
	balance *Balance
	id      string
}

/*
Bind attaches a reservation id after BuyAfter seeds the lot.
*/
func (claim *Claim) Bind(balance *Balance, id string) {
	if claim == nil {
		return
	}

	claim.balance = balance
	claim.id = id
}

/*
ID returns the live reservation id, if any.
*/
func (claim *Claim) ID() string {
	if claim == nil {
		return ""
	}

	return claim.id
}

/*
Funded reports whether the claim still covers amount.
*/
func (claim *Claim) Funded(amount *decimal.Decimal) bool {
	if claim == nil || claim.balance == nil || claim.id == "" {
		return false
	}

	return claim.balance.Funded(claim.id, amount)
}

/*
Release returns Booked cash when entry fails or cancels before fill Consume.
*/
func (claim *Claim) Release() {
	if claim == nil || claim.balance == nil || claim.id == "" {
		return
	}

	_ = claim.balance.Release(claim.id)
	claim.id = ""
}

/*
Consume drops the ledger entry after a buy fill so later cancel cannot
double-credit Available.
*/
func (claim *Claim) Consume() {
	if claim == nil || claim.balance == nil || claim.id == "" {
		return
	}

	claim.balance.Consume(claim.id)
	claim.id = ""
}

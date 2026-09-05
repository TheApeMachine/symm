package cmd

import (
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
	cashReservation remains charged until a balance request begun after its

terminal execution has returned. A stale REST response cannot spend it twice.
*/
type cashReservation struct {
	cost     *big.Rat
	terminal time.Time
}

/*
	accountFunds owns the atomic check-and-reserve boundary and publishes immutable

account inputs. Exchange balances are never locally edited. This mutex serializes
REST publication, the order worker and execution callbacks, not market processing.
*/
type accountFunds struct {
	mu           sync.Mutex
	source       atomic.Pointer[types.EquityReading]
	state        atomic.Pointer[strategy.AccountState]
	reservations map[string]cashReservation
}

/* Observe publishes each producer mark once; repeated market deliveries are reads. */
func (funds *accountFunds) Observe(reading *types.EquityReading) strategy.AccountState {
	if reading == nil {
		return strategy.AccountState{Reason: "awaiting authoritative account mark"}
	}

	if funds.source.Load() != reading {
		funds.mu.Lock()

		if previous := funds.source.Load(); previous == nil || reading.Version > previous.Version {
			for identity, reservation := range funds.reservations {
				if !reservation.terminal.IsZero() && reading.From.After(reservation.terminal) {
					delete(funds.reservations, identity)
				}
			}
			funds.publish(reading)
			funds.source.Store(reading)
		}
		funds.mu.Unlock()
	}
	return *funds.state.Load()
}

/* Reserve subtracts one fee-inclusive commitment atomically from available cash. */
func (funds *accountFunds) Reserve(identity string, cost *big.Rat) bool {
	funds.mu.Lock()
	defer funds.mu.Unlock()
	state := funds.state.Load()

	if state == nil || !state.Complete || cost == nil || cost.Sign() <= 0 {
		return false
	}

	if _, exists := funds.reservations[identity]; exists {
		return false
	}
	cash, valid := new(big.Rat).SetString(state.Cash)

	if !valid {
		panic("funds: invalid authoritative cash")
	}

	if cost.Cmp(cash) > 0 {
		return false
	}

	if funds.reservations == nil {
		funds.reservations = make(map[string]cashReservation)
	}
	funds.reservations[identity] = cashReservation{cost: new(big.Rat).Set(cost)}
	funds.publish(funds.source.Load())
	return true
}

/*
	Release clears a proven pre-submission refusal or records a terminal venue fact.

A submitted order retains its reservation until a causally later balance refresh.
*/
func (funds *accountFunds) Release(identity string, terminal time.Time) {
	funds.mu.Lock()
	defer funds.mu.Unlock()
	reservation, found := funds.reservations[identity]

	if !found {
		return
	}

	if terminal.IsZero() {
		delete(funds.reservations, identity)
		funds.publish(funds.source.Load())
		return
	}
	reservation.terminal = terminal
	funds.reservations[identity] = reservation
}

/* publish computes exact free cash from the authoritative balance and open reservations. */
func (funds *accountFunds) publish(reading *types.EquityReading) {
	state := strategy.AccountState{Cash: reading.AvailableCash, ActualCash: reading.Cash, Positions: reading.Positions, Complete: reading.Complete, Reason: reading.FundingReason}
	equity, err := strconv.ParseFloat(reading.Equity, 64)

	if err != nil {
		panic(err)
	}
	state.Mark = strategy.EquityMark{At: reading.At, Version: reading.Version, Equity: equity}

	if reading.NetFunding != "" {
		funding, err := strconv.ParseFloat(reading.NetFunding, 64)

		if err != nil {
			panic(err)
		}
		state.Mark.NetFunding, state.Mark.HasFunding = funding, true
	}

	if !state.Mark.HasFunding {
		state.Complete = false

		if state.Reason == "" {
			state.Reason = "funding information unavailable"
		}
	}
	cash, valid := new(big.Rat).SetString(reading.AvailableCash)

	if !valid {
		state.Complete = false
		state.Reason = "available quote balance unavailable"
		funds.state.Store(&state)
		return
	}
	committed := new(big.Rat)
	for _, reservation := range funds.reservations {
		committed.Add(committed, reservation.cost)
	}
	// The venue may already hold some local reservations. Keeping them charged
	// until terminal reconciliation is conservative while that overlap is unknown.
	total, valid := new(big.Rat).SetString(reading.Cash)

	if !valid {
		panic("funds: invalid authoritative total quote cash")
	}

	state.Committed = new(big.Rat).Add(total.Sub(total, cash), committed).RatString()
	state.Cash = cash.Sub(cash, committed).RatString()
	funds.state.Store(&state)
}

package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
TestClaimLeakWhenLotOpensFromSnapshot probes the seam between the local claim
ledger and the wallet snapshot: when the venue confirms a buy through a balances
snapshot (quote down, base up) but the matching execution never reaches the
position, is the entry claim reconciled away, or does it leak and permanently
shrink effective available cash?
*/
func TestClaimLeakWhenLotOpensFromSnapshot(t *testing.T) {
	balance := NewBalance(nil, nil, nil)
	balance.status = types.READY
	balance.quote = "USD"
	balance.model = &kraken.Balance{Data: []kraken.BalanceData{{
		Asset:     "USD",
		Available: decimal.NewFromFloat64(1000),
		Reserved:  decimal.NewFromFloat64(0),
	}}}

	claim, err := balance.Book(decimal.NewFromFloat64(100), nil, "MATIC/USD")

	if err != nil || claim == nil {
		t.Fatalf("Book failed: %v %#v", err, claim)
	}

	// The venue fills the buy: USD down to 900, MATIC up to 1. The execution
	// frame that would call claim.Consume() never arrives at any Position.
	balance.BalanceAck([]byte(`{"channel":"balances","type":"snapshot","sequence":2,"data":[
		{"asset":"USD","balance":"900","available":"900","reserved":"0"},
		{"asset":"MATIC","balance":"1","available":"1","reserved":"0"}
	]}`))

	cash, cashErr := balance.AvailableCash()

	if cashErr != nil {
		t.Fatalf("available cash: %v", cashErr)
	}

	leaked := balance.Funded(claim.ID, decimal.NewFromFloat64(1))
	t.Logf("after snapshot-opened lot without execution: availableCash=%v claimStillFunded=%v",
		cash, leaked)

	// The venue snapshot already reflects the spend (USD 900). Correct behavior
	// is availableCash == 900. If the claim leaks, it double-counts to 800.
	if cash.Float64() != 900 {
		t.Fatalf("CLAIM LEAK: entry claim not reconciled by wallet snapshot; "+
			"availableCash want 900 got %v (claim still funded=%v)", cash, leaked)
	}
}

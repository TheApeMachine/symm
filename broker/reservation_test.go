package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestBookReleaseConsume(t *testing.T) {
	t.Parallel()

	balance := NewBalance(nil, nil, nil)
	balance.status = types.READY
	balance.model = &kraken.Balance{
		Data: []kraken.BalanceData{{
			Asset:     "USD",
			Available: decimal.NewFromFloat64(1000),
			Reserved:  decimal.NewFromFloat64(0),
		}},
	}
	balance.quote = "USD"

	claim, err := balance.Book(decimal.NewFromFloat64(100), nil)

	if err != nil || claim == nil || claim.ID == "" {
		t.Fatalf("Book failed: %v %#v", err, claim)
	}

	if !balance.Funded(claim.ID, decimal.NewFromFloat64(100)) {
		t.Fatal("Funded should cover booked amount")
	}

	if err := balance.Release(claim.ID); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	if balance.Funded(claim.ID, decimal.NewFromFloat64(1)) {
		t.Fatal("Funded after Release must be false")
	}

	claim, err = balance.Book(decimal.NewFromFloat64(50), nil)

	if err != nil {
		t.Fatalf("re-Book failed: %v", err)
	}

	balance.Consume(claim.ID)

	if balance.Funded(claim.ID, decimal.NewFromFloat64(1)) {
		t.Fatal("Funded after Consume must be false")
	}
}

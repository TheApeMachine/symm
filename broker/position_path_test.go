package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
)

func TestPositionRefusesBlankPath(t *testing.T) {
	position := NewPosition("BTC/USD", "", true, nil)

	if _, err := position.Enter(""); err == nil {
		t.Fatal("enter accepted a blank path")
	}
	if !position.OpenedAt.IsZero() {
		t.Fatal("refused enter still opened the lot")
	}

	position.ExitOrders = []*spot.OrderRequest{{}}
	opened := time.Now().UTC()
	position.OpenedAt = opened

	if _, err := position.Exit(""); err == nil {
		t.Fatal("exit accepted a blank path")
	}
	if position.ClosedAt != nil {
		t.Fatal("refused exit still closed the lot")
	}
	if position.ExitPath != "" {
		t.Fatal("refused exit stored an exit path")
	}
}

package broker

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
)

type Balances struct {
	desk *Desk
	ui   chan []byte
}

func NewBalances(desk *Desk, ui chan []byte) *Balances {
	return &Balances{
		desk: desk,
		ui:   ui,
	}
}

func (balances *Balances) On(data []byte) {
	balances.desk.balance = kraken.NewBalanceDataSlice(data)
	balances.desk.refreshStatus()

	if balances.ui == nil || balances.desk.balance == nil {
		return
	}

	if balances.ui != nil {
		select {
		case balances.ui <- datura.Map[any]{
			"balances": *balances.desk.balance,
		}.Marshal():
		default:
		}
	}
}

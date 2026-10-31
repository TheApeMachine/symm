package broker

import (
	"sync"
	"time"
)

// MoverRefresh is how often Price re-pulls public Ticker into the mover.
const MoverRefresh = time.Minute

/*
Mover holds the latest public Ticker 24h quote volume (and open-to-last
change) per symbol so quiet books never reach evaluation.
*/
type Mover struct {
	by sync.Map
}

type moverSnap struct {
	volumeUSD float64
	changePct float64
}

func NewMover() *Mover {
	return &Mover{}
}

/*
Apply stores one symbol's ticker snapshot (24h quote notional + change %).
*/
func (mover *Mover) Apply(symbol string, volumeUSD, changePct float64) {
	if mover == nil || symbol == "" {
		return
	}

	mover.by.Store(symbol, moverSnap{
		volumeUSD: volumeUSD,
		changePct: changePct,
	})
}

/*
VolumeUSD is the last Ticker 24h quote notional for the symbol.
*/
func (mover *Mover) VolumeUSD(symbol string) float64 {
	if mover == nil || symbol == "" {
		return 0
	}

	raw, ok := mover.by.Load(symbol)
	if !ok {
		return 0
	}

	return raw.(moverSnap).volumeUSD
}

/*
ChangePct is the last Ticker open-to-last percent move for the symbol.
*/
func (mover *Mover) ChangePct(symbol string) float64 {
	if mover == nil || symbol == "" {
		return 0
	}

	raw, ok := mover.by.Load(symbol)
	if !ok {
		return 0
	}

	return raw.(moverSnap).changePct
}

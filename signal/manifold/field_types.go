package manifold

import (
	"sync"
	"time"

	mkernel "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/signal/compute"
)

/*
Field owns the shared GPU manifold solver and projects the live universe into it.
Solver mutations run only on the bound SerialPool worker; readings publish through sync.Map.
*/
type Field struct {
	config                 mkernel.Config
	solver                 *mkernel.Solver
	universe               *Universe
	lastStepAt             time.Time
	lastIntegratedCarriers int
	lastReading            mkernel.Reading
	lastCarriers           []fieldCarrier
	readings               sync.Map
	pendingDeposits        []cellDeposit
	pendingWhales          []whaleCarrier
	activeWhales           []whaleCarrier
	lastRecreateAt         time.Time
	measurementsCapacity   int
	serial                 *compute.SerialPool
}

type whaleCarrier struct {
	symbol     string
	oscillator mkernel.Oscillator
}

type fieldCarrier struct {
	role       string
	symbol     string
	oscillator mkernel.Oscillator
}

type cellDeposit struct {
	cellX uint32
	cellY uint32
	cellZ uint32
	rho   float64
	momX  float64
	momY  float64
	momZ  float64
	eInt  float64
}

type symbolReading struct {
	reading mkernel.Reading
	price   float64
	at      time.Time
}

package parallel

import (
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
BookID labels one virtual experiment desk. A, B, and C share fee-net rules
but never share positions with each other or with the incumbent strategy.
*/
type BookID string

const (
	BookControl   BookID = "A"
	BookTreatment BookID = "B"
	BookPhysics   BookID = "C"
)

type Lot struct {
	ID        string
	Symbol    string
	Book      BookID
	EntryPath string
	Qty       *decimal.Decimal
	Entry     *decimal.Decimal
	Fee       *decimal.Decimal
	OpenedAt  time.Time
}

type ClosedLot struct {
	Lot
	ExitPath string
	Return   float64
	ClosedAt time.Time
}

type BookStats struct {
	Closed     int     `json:"closed"`
	Open       int     `json:"open"`
	Wins       int     `json:"wins"`
	WinRate    float64 `json:"winRate"`
	MeanReturn float64 `json:"meanReturn"`
}

type Ledger struct {
	mu      sync.Mutex
	open    map[BookID]map[string]*Lot
	closed  []ClosedLot
	returns map[BookID][]float64
}

func NewLedger() *Ledger {
	open := map[BookID]map[string]*Lot{
		BookControl:   {},
		BookTreatment: {},
		BookPhysics:   {},
	}
	returns := map[BookID][]float64{
		BookControl:   {},
		BookTreatment: {},
		BookPhysics:   {},
	}

	return &Ledger{
		open:    open,
		returns: returns,
	}
}

func (ledger *Ledger) Open(book BookID, lot *Lot) {
	if ledger == nil || lot == nil {
		return
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	if ledger.open[book] == nil {
		ledger.open[book] = map[string]*Lot{}
	}

	if lot.OpenedAt.IsZero() {
		lot.OpenedAt = time.Now().UTC()
	}

	ledger.open[book][lot.Symbol] = lot
}

func (ledger *Ledger) Close(book BookID, symbol, exitPath string, ret float64) {
	if ledger == nil {
		return
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	lot := ledger.open[book][symbol]
	if lot == nil {
		return
	}

	delete(ledger.open[book], symbol)
	now := time.Now().UTC()

	ledger.closed = append(ledger.closed, ClosedLot{
		Lot:      *lot,
		ExitPath: exitPath,
		Return:   ret,
		ClosedAt: now,
	})

	ledger.returns[book] = append(ledger.returns[book], ret)
}

func (ledger *Ledger) Stats(book BookID) BookStats {
	if ledger == nil {
		return BookStats{}
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	rets := ledger.returns[book]
	openN := len(ledger.open[book])

	if len(rets) == 0 {
		return BookStats{Open: openN}
	}

	wins := 0
	sum := 0.0

	for _, ret := range rets {
		sum += ret
		if ret > 0 {
			wins++
		}
	}

	return BookStats{
		Closed:     len(rets),
		Open:       openN,
		Wins:       wins,
		WinRate:    float64(wins) / float64(len(rets)),
		MeanReturn: sum / float64(len(rets)),
	}
}

func (ledger *Ledger) HasOpen(book BookID, symbol string) bool {
	if ledger == nil {
		return false
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	_, ok := ledger.open[book][symbol]

	return ok
}

func (ledger *Ledger) OpenLot(book BookID, symbol string) *Lot {
	if ledger == nil {
		return nil
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	return ledger.open[book][symbol]
}

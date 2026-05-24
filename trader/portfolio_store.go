package trader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

/*
PortfolioStore persists open positions and wallet state across restarts.
*/
type PortfolioStore struct {
	mu   sync.Mutex
	path string
}

/*
PortfolioSnapshot is the on-disk portfolio state.
*/
type PortfolioSnapshot struct {
	CashEUR        float64            `json:"cash_eur"`
	ReservedEUR    float64            `json:"reserved_eur"`
	Inventory      map[string]float64 `json:"inventory,omitempty"`
	ClosedPnLEUR   float64            `json:"closed_pnl_eur"`
	DailyClosedPnL float64            `json:"daily_closed_pnl_eur"`
	DailyPnLDay    time.Time          `json:"daily_pnl_day"`
	TradeCount     int                `json:"trade_count"`
	Wins           int                `json:"wins"`
	HaltReason     string             `json:"halt_reason,omitempty"`
	Positions      []PositionRecord   `json:"positions,omitempty"`
}

/*
PositionRecord is one serializable open position.
*/
type PositionRecord struct {
	Symbol      string    `json:"symbol"`
	Source      string    `json:"source,omitempty"`
	Regime      string    `json:"regime,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Score       float64   `json:"score,omitempty"`
	Side        int       `json:"side"`
	EntryPrice  float64   `json:"entry_price"`
	FillPrice   float64   `json:"fill_price"`
	StopPrice   float64   `json:"stop_price"`
	PeakPrice   float64   `json:"peak_price"`
	NotionalEUR float64   `json:"notional_eur"`
	EntryFeeEUR float64   `json:"entry_fee_eur"`
	TrailPct    float64   `json:"trail_pct"`
	BaseQty     float64   `json:"base_qty"`
	OrderID     string    `json:"order_id,omitempty"`
	StopOrderID string    `json:"stop_order_id,omitempty"`
	OpenedAt    time.Time `json:"opened_at"`
}

/*
NewPortfolioStore creates a portfolio snapshot store under logDir.
*/
func NewPortfolioStore(logDir string) *PortfolioStore {
	path := ""

	if logDir != "" {
		path = filepath.Join(logDir, "portfolio.json")
	}

	return &PortfolioStore{path: path}
}

/*
Restore loads one saved snapshot into a paper portfolio.
*/
func (store *PortfolioStore) Restore(portfolio *Portfolio) error {
	if store == nil || portfolio == nil || store.path == "" {
		return nil
	}

	if portfolio.wallet != nil && portfolio.wallet.Type != PaperWallet {
		return nil
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	payload, err := os.ReadFile(store.path)

	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("read portfolio snapshot: %w", err)
	}

	var snapshot PortfolioSnapshot

	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return fmt.Errorf("decode portfolio snapshot: %w", err)
	}

	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	return portfolio.applySnapshotLocked(snapshot)
}

/*
Save writes the current portfolio state to disk.
*/
func (store *PortfolioStore) Save(portfolio *Portfolio) error {
	if store == nil || portfolio == nil || store.path == "" {
		return nil
	}

	portfolio.mu.Lock()
	snapshot := portfolio.snapshotLocked()
	portfolio.mu.Unlock()

	payload, err := json.Marshal(snapshot)

	if err != nil {
		return fmt.Errorf("encode portfolio snapshot: %w", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		return fmt.Errorf("create portfolio dir: %w", err)
	}

	tempPath := store.path + ".tmp"

	if err := os.WriteFile(tempPath, payload, 0o600); err != nil {
		return fmt.Errorf("write portfolio snapshot: %w", err)
	}

	if err := os.Rename(tempPath, store.path); err != nil {
		return fmt.Errorf("commit portfolio snapshot: %w", err)
	}

	return nil
}

func (portfolio *Portfolio) snapshotLocked() PortfolioSnapshot {
	snapshot := PortfolioSnapshot{
		ClosedPnLEUR:   portfolio.closedPnL,
		DailyClosedPnL: portfolio.dailyClosedPnL,
		DailyPnLDay:    portfolio.dailyPnLDay,
		TradeCount:     portfolio.tradeCount,
		Wins:           portfolio.wins,
		HaltReason:     portfolio.haltReason,
		Positions:      make([]PositionRecord, 0, len(portfolio.positions)),
	}

	if portfolio.wallet != nil {
		snapshot.CashEUR = portfolio.wallet.Balance
		snapshot.ReservedEUR = portfolio.wallet.ReservedEUR
		snapshot.Inventory = copyInventory(portfolio.wallet.Inventory)
	}

	for symbol, position := range portfolio.positions {
		snapshot.Positions = append(snapshot.Positions, positionRecordFrom(symbol, position))
	}

	return snapshot
}

func (portfolio *Portfolio) applySnapshotLocked(snapshot PortfolioSnapshot) error {
	if portfolio.wallet == nil {
		return fmt.Errorf("wallet is required")
	}

	portfolio.wallet.Balance = snapshot.CashEUR
	portfolio.wallet.ReservedEUR = snapshot.ReservedEUR
	portfolio.wallet.Inventory = copyInventory(snapshot.Inventory)
	portfolio.closedPnL = snapshot.ClosedPnLEUR
	portfolio.dailyClosedPnL = snapshot.DailyClosedPnL
	portfolio.dailyPnLDay = snapshot.DailyPnLDay
	portfolio.tradeCount = snapshot.TradeCount
	portfolio.wins = snapshot.Wins
	portfolio.haltReason = snapshot.HaltReason
	portfolio.positions = make(map[string]*Position, len(snapshot.Positions))

	for _, record := range snapshot.Positions {
		if record.Symbol == "" {
			continue
		}

		portfolio.positions[record.Symbol] = positionFromRecord(record)
	}

	return nil
}

func copyInventory(source map[string]float64) map[string]float64 {
	if len(source) == 0 {
		return make(map[string]float64)
	}

	copied := make(map[string]float64, len(source))

	for symbol, qty := range source {
		if qty <= 0 {
			continue
		}

		copied[symbol] = qty
	}

	return copied
}

func positionRecordFrom(symbol string, position *Position) PositionRecord {
	if position == nil {
		return PositionRecord{Symbol: symbol}
	}

	return PositionRecord{
		Symbol:      position.Symbol,
		Source:      position.Source,
		Regime:      position.Regime,
		Reason:      position.Reason,
		Score:       position.Score,
		Side:        position.Side,
		EntryPrice:  position.EntryPrice,
		FillPrice:   position.FillPrice,
		StopPrice:   position.StopPrice,
		PeakPrice:   position.PeakPrice,
		NotionalEUR: position.NotionalEUR,
		EntryFeeEUR: position.EntryFeeEUR,
		TrailPct:    position.TrailPct,
		BaseQty:     position.BaseQty,
		OrderID:     position.OrderID,
		StopOrderID: position.StopOrderID,
		OpenedAt:    position.OpenedAt,
	}
}

func positionFromRecord(record PositionRecord) *Position {
	return &Position{
		Symbol:      record.Symbol,
		Source:      record.Source,
		Regime:      record.Regime,
		Reason:      record.Reason,
		Score:       record.Score,
		Side:        record.Side,
		EntryPrice:  record.EntryPrice,
		FillPrice:   record.FillPrice,
		StopPrice:   record.StopPrice,
		PeakPrice:   record.PeakPrice,
		NotionalEUR: record.NotionalEUR,
		EntryFeeEUR: record.EntryFeeEUR,
		TrailPct:    record.TrailPct,
		BaseQty:     record.BaseQty,
		OrderID:     record.OrderID,
		StopOrderID: record.StopOrderID,
		OpenedAt:    record.OpenedAt,
	}
}

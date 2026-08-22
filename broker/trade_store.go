package broker

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

const positionTradesSchema = `
CREATE TABLE IF NOT EXISTS position_trades (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol TEXT NOT NULL,
    status TEXT NOT NULL,
    decision_id TEXT NOT NULL,
    entry_at TEXT,
    entry_price TEXT,
    entry_fee TEXT,
    qty TEXT,
    exit_at TEXT,
    exit_price TEXT,
    exit_fee TEXT,
    pnl TEXT,
    return_pct REAL,
    trigger_reason TEXT,
    trigger_mark TEXT,
    floor TEXT,
    peak TEXT,
    profit_line TEXT,
    locked INTEGER,
    thesis_score REAL,
    graph_score REAL,
    cause TEXT,
    raw_position BLOB NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;
CREATE UNIQUE INDEX IF NOT EXISTS uidx_position_trades_decision ON position_trades(decision_id);
CREATE INDEX IF NOT EXISTS idx_position_trades_symbol ON position_trades(symbol);
`

/*
SaveTrade writes or updates a position's complete lifecycle and economics in the trade journal table.
*/
func (store *PositionStore) SaveTrade(position *Position) error {
	if store == nil || store.database == nil || position == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position store: valid position required to save trade",
			nil,
		))
	}

	rawPosition, err := json.Marshal(position.Wire())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: marshal trade wire failed [%s]", err.Error()),
			err,
		))
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	decisionID := position.Decision.ID

	if decisionID == "" {
		decisionID = position.pair.Symbol + ":" + now
	}

	var entryAt *string
	var entryPrice *string
	var entryFee *string
	var qty *string
	var exitAt *string
	var exitPrice *string
	var exitFee *string
	var pnl *string
	var triggerReason *string
	var triggerMark *string
	var floor *string
	var peak *string
	var profitLine *string
	locked := 0
	returnPct := 0.0

	if position.Holding != nil {
		holding := position.Holding
		returnPct = holding.ReturnPct

		if holding.EntryAt != nil && !holding.EntryAt.IsZero() {
			formatted := holding.EntryAt.UTC().Format(time.RFC3339Nano)
			entryAt = &formatted
		}

		if holding.EntryPrice != nil {
			value := holding.EntryPrice.String()
			entryPrice = &value
		}

		if holding.EntryFee != nil {
			value := holding.EntryFee.String()
			entryFee = &value
		}

		if holding.Qty != nil {
			value := holding.Qty.String()
			qty = &value
		}

		if holding.ExitAt != nil && !holding.ExitAt.IsZero() {
			formatted := holding.ExitAt.UTC().Format(time.RFC3339Nano)
			exitAt = &formatted
		}

		if holding.ExitPrice != nil {
			value := holding.ExitPrice.String()
			exitPrice = &value
		}

		if holding.ExitFee != nil {
			value := holding.ExitFee.String()
			exitFee = &value
		}

		if holding.PnL != nil {
			value := holding.PnL.String()
			pnl = &value
		}

		if holding.Stoploss != nil {
			stoploss := holding.Stoploss

			if stoploss.TriggerReason != "" {
				triggerReason = &stoploss.TriggerReason
			}

			if stoploss.TriggerMark != nil {
				value := stoploss.TriggerMark.String()
				triggerMark = &value
			}

			if stoploss.Floor != nil {
				value := stoploss.Floor.String()
				floor = &value
			}

			if stoploss.Peak != nil {
				value := stoploss.Peak.String()
				peak = &value
			}

			if stoploss.ProfitLine != nil {
				value := stoploss.ProfitLine.String()
				profitLine = &value
			}

			if stoploss.Locked {
				locked = 1
			}
		}
	}

	query := `
INSERT INTO position_trades (
	symbol, status, decision_id, entry_at, entry_price, entry_fee, qty,
	exit_at, exit_price, exit_fee, pnl, return_pct, trigger_reason, trigger_mark,
	floor, peak, profit_line, locked, thesis_score, graph_score, cause,
	raw_position, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(decision_id) DO UPDATE SET
	status = excluded.status,
	entry_at = coalesce(excluded.entry_at, position_trades.entry_at),
	entry_price = coalesce(excluded.entry_price, position_trades.entry_price),
	entry_fee = coalesce(excluded.entry_fee, position_trades.entry_fee),
	qty = coalesce(excluded.qty, position_trades.qty),
	exit_at = coalesce(excluded.exit_at, position_trades.exit_at),
	exit_price = coalesce(excluded.exit_price, position_trades.exit_price),
	exit_fee = coalesce(excluded.exit_fee, position_trades.exit_fee),
	pnl = coalesce(excluded.pnl, position_trades.pnl),
	return_pct = excluded.return_pct,
	trigger_reason = coalesce(excluded.trigger_reason, position_trades.trigger_reason),
	trigger_mark = coalesce(excluded.trigger_mark, position_trades.trigger_mark),
	floor = coalesce(excluded.floor, position_trades.floor),
	peak = coalesce(excluded.peak, position_trades.peak),
	profit_line = coalesce(excluded.profit_line, position_trades.profit_line),
	locked = excluded.locked,
	thesis_score = excluded.thesis_score,
	graph_score = excluded.graph_score,
	cause = excluded.cause,
	raw_position = excluded.raw_position,
	updated_at = excluded.updated_at`

	_, err = store.database.Exec(
		query,
		position.pair.Symbol,
		string(position.status()),
		decisionID,
		entryAt,
		entryPrice,
		entryFee,
		qty,
		exitAt,
		exitPrice,
		exitFee,
		pnl,
		returnPct,
		triggerReason,
		triggerMark,
		floor,
		peak,
		profitLine,
		locked,
		position.Decision.ThesisScore,
		position.Decision.GraphScore,
		position.Decision.Cause,
		rawPosition,
		now,
		now,
	)

	if isNoSuchTable(err) {
		if schemaErr := store.EnsureSchema(); schemaErr != nil {
			return schemaErr
		}

		_, err = store.database.Exec(
			query,
			position.pair.Symbol,
			string(position.status()),
			decisionID,
			entryAt,
			entryPrice,
			entryFee,
			qty,
			exitAt,
			exitPrice,
			exitFee,
			pnl,
			returnPct,
			triggerReason,
			triggerMark,
			floor,
			peak,
			profitLine,
			locked,
			position.Decision.ThesisScore,
			position.Decision.GraphScore,
			position.Decision.Cause,
			rawPosition,
			now,
			now,
		)
	}

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: save trade failed for %s [%s]", position.pair.Symbol, err.Error()),
			err,
		))
	}

	return nil
}

/*
RecentTrades retrieves the most recent trade journal records.
*/
func (store *PositionStore) RecentTrades(limit int) ([]*wire.PositionT, error) {
	if store == nil || store.database == nil {
		return nil, nil
	}

	if limit <= 0 {
		limit = 100
	}

	rows, err := store.database.Query(
		"SELECT raw_position FROM position_trades ORDER BY id DESC LIMIT ?",
		limit,
	)

	if isNoSuchTable(err) {
		if schemaErr := store.EnsureSchema(); schemaErr != nil {
			return nil, schemaErr
		}

		return nil, nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("position store: recent trades query failed [%s]", err.Error()),
			err,
		))
	}

	defer rows.Close()
	trades := make([]*wire.PositionT, 0)

	for rows.Next() {
		var raw []byte

		if scanErr := rows.Scan(&raw); scanErr != nil {
			continue
		}

		var positionWire wire.PositionT

		if unmarshalErr := json.Unmarshal(raw, &positionWire); unmarshalErr != nil {
			continue
		}

		trades = append(trades, &positionWire)
	}

	return trades, rows.Err()
}

import type { AuditEvent } from "#/lib/symm/events";
import { isAuditEvent, walletPayloadFromFrame } from "#/lib/symm/events";

export type TradeHistoryOutcome = "profit" | "loss" | "flat";

export type TradeHistoryRow = {
	key: string;
	symbol: string;
	qty?: number;
	entryPrice?: number;
	exitPrice?: number;
	realizedEur: number;
	realizedPct?: number;
	outcome: TradeHistoryOutcome;
	reason?: string;
	closedAt: string;
};

type OpenTrade = {
	symbol: string;
	qty: number;
	entryPrice: number;
	markPrice?: number;
	unrealizedEur?: number;
	unrealizedPct?: number;
	openedAt: string;
};

type PositionSnapshot = {
	symbol: string;
	qty: number;
	entryPrice: number;
	markPrice?: number;
	unrealizedEur?: number;
	unrealizedPct?: number;
};

type Listener = () => void;

const MAX_ROWS = 120;

const outcomeFromValue = (value: number): TradeHistoryOutcome => {
	if (value > 0) {
		return "profit";
	}

	if (value < 0) {
		return "loss";
	}

	return "flat";
};

const nowIso = () => new Date().toISOString();

const finiteNumber = (value: unknown): number | undefined => {
	if (typeof value !== "number" || !Number.isFinite(value)) {
		return undefined;
	}

	return value;
};

const positionFromMonitorRow = (value: unknown): PositionSnapshot | null => {
	if (typeof value !== "object" || value === null || Array.isArray(value)) {
		return null;
	}

	const record = value as Record<string, unknown>;
	const symbol = typeof record.symbol === "string" ? record.symbol.trim() : "";
	const qty = finiteNumber(record.qty);
	const entryPrice = finiteNumber(record.avg_entry) ?? 0;
	const markPrice = finiteNumber(record.mark);
	const unrealizedEur = finiteNumber(record.unrealized);
	const unrealizedPct = finiteNumber(record.unrealized_pct);

	if (symbol === "" || qty === undefined || qty <= 0) {
		return null;
	}

	return {
		symbol,
		qty,
		entryPrice,
		markPrice,
		unrealizedEur,
		unrealizedPct,
	};
};

/*
TradeHistoryDataProvider records closed trades and their final profit or loss.
Inventory drops and audit exit events both finalize rows.
*/
class TradeHistoryDataProviderImpl {
	private openBySymbol = new Map<string, OpenTrade>();
	private rows: readonly TradeHistoryRow[] = [];
	private listeners = new Set<Listener>();

	subscribe(listener: Listener) {
		this.listeners.add(listener);

		return () => {
			this.listeners.delete(listener);
		};
	}

	snapshot(): readonly TradeHistoryRow[] {
		return this.rows;
	}

	private notify() {
		for (const listener of this.listeners) {
			listener();
		}
	}

	private pushRow(row: TradeHistoryRow) {
		this.rows = [row, ...this.rows].slice(0, MAX_ROWS);
		this.notify();
	}

	private syncOpenPositions(positions: PositionSnapshot[]) {
		const seen = new Set<string>();

		for (const position of positions) {
			seen.add(position.symbol);

			const existing = this.openBySymbol.get(position.symbol);

			if (existing === undefined) {
				this.openBySymbol.set(position.symbol, {
					symbol: position.symbol,
					qty: position.qty,
					entryPrice: position.entryPrice,
					markPrice: position.markPrice,
					unrealizedEur: position.unrealizedEur,
					unrealizedPct: position.unrealizedPct,
					openedAt: nowIso(),
				});
				continue;
			}

			existing.qty = position.qty;
			existing.entryPrice = position.entryPrice;
			existing.markPrice = position.markPrice;
			existing.unrealizedEur = position.unrealizedEur;
			existing.unrealizedPct = position.unrealizedPct;
		}

		for (const [symbol, openTrade] of this.openBySymbol.entries()) {
			if (seen.has(symbol)) {
				continue;
			}

			this.openBySymbol.delete(symbol);
			this.finalizeOpenTrade(openTrade, openTrade.unrealizedEur ?? 0, {
				realizedPct: openTrade.unrealizedPct,
				reason: "position closed",
				closedAt: nowIso(),
			});
		}
	}

	private finalizeOpenTrade(
		openTrade: OpenTrade,
		realizedEur: number,
		details: {
			realizedPct?: number;
			exitPrice?: number;
			reason?: string;
			closedAt: string;
		},
	) {
		const exitPrice =
			details.exitPrice ??
			(openTrade.qty > 0
				? openTrade.entryPrice + realizedEur / openTrade.qty
				: undefined);

		this.pushRow({
			key: `${openTrade.symbol}:${details.closedAt}`,
			symbol: openTrade.symbol,
			qty: openTrade.qty,
			entryPrice: openTrade.entryPrice,
			exitPrice,
			realizedEur,
			realizedPct: details.realizedPct,
			outcome: outcomeFromValue(realizedEur),
			reason: details.reason,
			closedAt: details.closedAt,
		});
	}

	private positionsFromBalance(
		raw: Record<string, unknown>,
	): PositionSnapshot[] {
		const payload = walletPayloadFromFrame(raw);
		const currency = payload.Currency ?? "EUR";
		const inventory = payload.Inventory ?? {};
		const avgEntry = payload.AvgEntry ?? {};
		const unrealized = payload.Unrealized ?? {};
		const marks = payload.Marks ?? {};
		const positions: PositionSnapshot[] = [];

		for (const [base, qty] of Object.entries(inventory)) {
			if (qty <= 0) {
				continue;
			}

			const symbol = `${base}/${currency}`;
			const entryPrice = avgEntry[base] ?? 0;
			const markPrice = marks[symbol];
			const unrealizedEur = unrealized[base];
			const entryCost = qty * entryPrice;
			const unrealizedPct =
				entryCost > 0 && unrealizedEur !== undefined
					? (unrealizedEur / entryCost) * 100
					: undefined;

			positions.push({
				symbol,
				qty,
				entryPrice,
				markPrice,
				unrealizedEur,
				unrealizedPct,
			});
		}

		return positions;
	}

	ingestBalance(raw: Record<string, unknown>) {
		this.syncOpenPositions(this.positionsFromBalance(raw));
	}

	ingestPositions(raw: Record<string, unknown>) {
		const rows = Array.isArray(raw.positions) ? raw.positions : [];
		const positions = rows
			.map((row) => positionFromMonitorRow(row))
			.filter((row): row is PositionSnapshot => row !== null);

		this.syncOpenPositions(positions);
	}

	ingestAudit(raw: unknown) {
		if (!isAuditEvent(raw)) {
			return;
		}

		const auditEvent = raw.audit_event;

		if (auditEvent === "entry" || auditEvent === "trade_entry_fill") {
			this.recordAuditEntry(raw);
			return;
		}

		if (auditEvent !== "exit" && auditEvent !== "trade_exit_fill") {
			return;
		}

		this.recordAuditExit(raw);
	}

	private recordAuditEntry(event: AuditEvent) {
		const symbol = event.symbol?.trim();

		if (symbol === undefined || symbol === "") {
			return;
		}

		const fillPrice = event.fill_price;
		const existing = this.openBySymbol.get(symbol);

		if (existing !== undefined) {
			if (fillPrice !== undefined && fillPrice > 0) {
				existing.entryPrice = fillPrice;
			}

			return;
		}

		this.openBySymbol.set(symbol, {
			symbol,
			qty: 0,
			entryPrice: fillPrice ?? 0,
			openedAt: event.ts,
		});
	}

	private recordAuditExit(event: AuditEvent) {
		const symbol = event.symbol?.trim();

		if (symbol === undefined || symbol === "") {
			return;
		}

		const openTrade = this.openBySymbol.get(symbol);
		const actualReturn = event.actual_return;
		const entryCost =
			openTrade !== undefined && openTrade.qty > 0 && openTrade.entryPrice > 0
				? openTrade.qty * openTrade.entryPrice
				: event.slot_eur;
		const realizedEur =
			actualReturn !== undefined && entryCost !== undefined
				? entryCost * actualReturn
				: (openTrade?.unrealizedEur ?? 0);
		const realizedPct =
			actualReturn !== undefined
				? actualReturn * 100
				: openTrade?.unrealizedPct;
		const exitPrice = event.fill_price;

		if (openTrade !== undefined) {
			this.openBySymbol.delete(symbol);
			this.finalizeOpenTrade(openTrade, realizedEur, {
				realizedPct,
				exitPrice,
				reason: event.reason ?? event.why,
				closedAt: event.ts,
			});
			return;
		}

		this.pushRow({
			key: `${symbol}:${event.seq}`,
			symbol,
			exitPrice,
			realizedEur,
			realizedPct,
			outcome:
				event.success === true
					? "profit"
					: event.success === false
						? "loss"
						: outcomeFromValue(realizedEur),
			reason: event.reason ?? event.why,
			closedAt: event.ts,
		});
	}

	reset() {
		this.openBySymbol.clear();
		this.rows = [];
		this.notify();
	}
}

const shared = createTradeHistoryDataProviderImpl();

export const createTradeHistoryDataProvider = () =>
	createTradeHistoryDataProviderImpl();

function createTradeHistoryDataProviderImpl() {
	const impl = new TradeHistoryDataProviderImpl();

	return {
		subscribe: (listener: Listener) => impl.subscribe(listener),
		snapshot: () => impl.snapshot(),
		ingestBalance: (raw: Record<string, unknown>) => impl.ingestBalance(raw),
		ingestPositions: (raw: Record<string, unknown>) =>
			impl.ingestPositions(raw),
		ingestAudit: (raw: unknown) => impl.ingestAudit(raw),
		reset: () => impl.reset(),
	};
}

export type TradeHistoryStore = ReturnType<
	typeof createTradeHistoryDataProvider
>;

export const TradeHistoryDataProvider = shared;

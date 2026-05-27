import type { ExecutionFill, WalletPayload } from "#/lib/symm/events";
import { isExecutionFill, isWalletPayload } from "#/lib/symm/events";

export type TradePanelRow = {
	key: string;
	kind: "enter" | "exit" | "open";
	symbol: string;
	side?: string;
	qty?: number;
	price?: number;
	notionalEur?: number;
	entryPrice?: number;
	markPrice?: number;
	unrealizedEur?: number;
	unrealizedPct?: number;
};

type Listener = () => void;

const MAX_ROWS = 24;

const positionEconomics = (
	symbol: string,
	base: string,
	qty: number,
	payload: WalletPayload,
) => {
	const currency = payload.Currency ?? "EUR";
	const entryPrice = payload.AvgEntry?.[base];
	const markPrice = payload.Marks?.[symbol];

	if (entryPrice === undefined || markPrice === undefined || qty <= 0) {
		return {
			entryPrice,
			markPrice,
			unrealizedEur: undefined,
			unrealizedPct: undefined,
		};
	}

	const unrealizedEur = qty * (markPrice - entryPrice);
	const unrealizedPct = ((markPrice - entryPrice) / entryPrice) * 100;

	return {
		entryPrice,
		markPrice,
		unrealizedEur,
		unrealizedPct,
	};
};

/*
TradesDataProvider lists recent fills and open inventory for the aside panel.
*/
class TradesDataProviderImpl {
	private rows: TradePanelRow[] = [];
	private listeners = new Set<Listener>();
	private markFallback = new Map<string, number>();

	subscribe(listener: Listener) {
		this.listeners.add(listener);

		return () => {
			this.listeners.delete(listener);
		};
	}

	snapshot(): readonly TradePanelRow[] {
		return this.rows;
	}

	setMark(symbol: string, markPrice: number) {
		if (markPrice <= 0) {
			return;
		}

		this.markFallback.set(symbol, markPrice);
		this.refreshOpenMarks();
	}

	private notify() {
		for (const listener of this.listeners) {
			listener();
		}
	}

	private prepend(row: TradePanelRow) {
		this.rows = [
			row,
			...this.rows.filter((entry) => entry.key !== row.key),
		].slice(0, MAX_ROWS);
		this.notify();
	}

	private refreshOpenMarks() {
		let changed = false;

		this.rows = this.rows.map((row) => {
			if (row.kind !== "open" || row.qty === undefined) {
				return row;
			}

			const markPrice = this.markFallback.get(row.symbol) ?? row.markPrice;

			if (markPrice === undefined || row.entryPrice === undefined) {
				return row;
			}

			const unrealizedEur = row.qty * (markPrice - row.entryPrice);
			const unrealizedPct =
				((markPrice - row.entryPrice) / row.entryPrice) * 100;

			if (
				row.markPrice === markPrice &&
				row.unrealizedEur === unrealizedEur &&
				row.unrealizedPct === unrealizedPct
			) {
				return row;
			}

			changed = true;

			return {
				...row,
				markPrice,
				unrealizedEur,
				unrealizedPct,
			};
		});

		if (changed) {
			this.notify();
		}
	}

	private syncInventory(payload: WalletPayload) {
		const inventory = payload.Inventory ?? {};
		const openRows: TradePanelRow[] = [];

		for (const [base, qty] of Object.entries(inventory)) {
			if (qty <= 0) {
				continue;
			}

			const symbol = `${base}/${payload.Currency ?? "EUR"}`;
			const economics = positionEconomics(symbol, base, qty, payload);
			const liveMark = this.markFallback.get(symbol);
			const markPrice = liveMark ?? economics.markPrice;

			let unrealizedEur = economics.unrealizedEur;
			let unrealizedPct = economics.unrealizedPct;

			if (markPrice !== undefined && economics.entryPrice !== undefined) {
				unrealizedEur = qty * (markPrice - economics.entryPrice);
				unrealizedPct =
					((markPrice - economics.entryPrice) / economics.entryPrice) * 100;
			}

			openRows.push({
				key: `open:${base}`,
				kind: "open",
				symbol,
				qty,
				entryPrice: economics.entryPrice,
				markPrice,
				unrealizedEur,
				unrealizedPct,
			});
		}

		const fills = this.rows.filter((row) => row.kind !== "open");
		this.rows = [...openRows, ...fills].slice(0, MAX_ROWS);
		this.notify();
	}

	ingestFill(fill: ExecutionFill) {
		const notionalEur = fill.Qty * fill.Price;

		this.prepend({
			key: `${fill.OrderID}:${fill.Side}:${fill.Price}`,
			kind: fill.Side === "sell" ? "exit" : "enter",
			symbol: fill.Symbol,
			side: fill.Side,
			qty: fill.Qty,
			price: fill.Price,
			notionalEur,
		});
	}

	ingest(raw: unknown) {
		if (isExecutionFill(raw)) {
			this.ingestFill(raw);
			return;
		}

		if (!isWalletPayload(raw)) {
			return;
		}

		this.syncInventory(raw);
	}
}

const shared = new TradesDataProviderImpl();

export const TradesDataProvider = {
	subscribe: (listener: Listener) => shared.subscribe(listener),
	snapshot: () => shared.snapshot(),
	ingest: (raw: unknown) => shared.ingest(raw),
	setMark: (symbol: string, markPrice: number) =>
		shared.setMark(symbol, markPrice),
};

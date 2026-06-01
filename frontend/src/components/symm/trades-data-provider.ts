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

const MAX_OPEN = 12;
const MAX_FILLS = 12;

const positionEconomics = (
	symbol: string,
	base: string,
	qty: number,
	payload: WalletPayload,
) => {
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
TradesDataProvider tracks open positions and recent fill history for the sidebar panel.
Open positions are driven by wallet snapshots; fills come from execution events.
*/
class TradesDataProviderImpl {
	private openRows: TradePanelRow[] = [];
	private fillRows: TradePanelRow[] = [];
	private panelRows: readonly TradePanelRow[] = [];
	private listeners = new Set<Listener>();
	private markFallback = new Map<string, number>();

	subscribe(listener: Listener) {
		this.listeners.add(listener);

		return () => {
			this.listeners.delete(listener);
		};
	}

	snapshot(): readonly TradePanelRow[] {
		return this.panelRows;
	}

	private rebuildPanelRows() {
		this.panelRows = [...this.openRows, ...this.fillRows];
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

	private refreshOpenMarks() {
		let changed = false;

		this.openRows = this.openRows.map((row) => {
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

		if (!changed) {
			return;
		}

		this.rebuildPanelRows();
		this.notify();
	}

	private syncInventory(payload: WalletPayload) {
		const inventory = payload.Inventory ?? {};
		const next: TradePanelRow[] = [];

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

			next.push({
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

		this.openRows = next.slice(0, MAX_OPEN);
		this.rebuildPanelRows();
		this.notify();
	}

	ingestFill(fill: ExecutionFill) {
		const kind: "enter" | "exit" = fill.Side === "buy" ? "enter" : "exit";

		const row: TradePanelRow = {
			key: `fill:${fill.OrderID}`,
			kind,
			symbol: fill.Symbol,
			side: fill.Side,
			qty: fill.Qty,
			price: fill.Price,
			notionalEur: fill.Qty * fill.Price,
		};

		// Prepend newest fill; drop oldest beyond cap.
		this.fillRows = [row, ...this.fillRows].slice(0, MAX_FILLS);
		this.rebuildPanelRows();
		this.notify();
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

	reset() {
		this.openRows = [];
		this.fillRows = [];
		this.panelRows = [];
		this.markFallback.clear();
		this.notify();
	}
}

const shared = createTradesDataProviderImpl();

export const createTradesDataProvider = () => createTradesDataProviderImpl();

function createTradesDataProviderImpl() {
	const impl = new TradesDataProviderImpl();

	return {
		subscribe: (listener: Listener) => impl.subscribe(listener),
		snapshot: () => impl.snapshot(),
		ingest: (raw: unknown) => impl.ingest(raw),
		setMark: (symbol: string, markPrice: number) =>
			impl.setMark(symbol, markPrice),
		reset: () => impl.reset(),
	};
}

export type TradesStore = ReturnType<typeof createTradesDataProvider>;

export const TradesDataProvider = shared;

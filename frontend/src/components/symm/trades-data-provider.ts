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
};

type Listener = () => void;

const MAX_ROWS = 24;

/*
TradesDataProvider lists recent fills and open inventory for the aside panel.
*/
class TradesDataProviderImpl {
	private rows: TradePanelRow[] = [];
	private listeners = new Set<Listener>();

	subscribe(listener: Listener) {
		this.listeners.add(listener);

		return () => {
			this.listeners.delete(listener);
		};
	}

	snapshot(): readonly TradePanelRow[] {
		return this.rows;
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

	private syncInventory(payload: WalletPayload) {
		const inventory = payload.Inventory ?? {};
		const openRows: TradePanelRow[] = [];

		for (const [base, qty] of Object.entries(inventory)) {
			if (qty <= 0) {
				continue;
			}

			openRows.push({
				key: `open:${base}`,
				kind: "open",
				symbol: `${base}/${payload.Currency ?? "EUR"}`,
				qty,
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
};

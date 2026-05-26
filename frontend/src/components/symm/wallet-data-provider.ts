import type { WalletPayload } from "#/lib/symm/events";
import { isWalletPayload } from "#/lib/symm/events";

export type WalletView = {
	currency: string;
	balance: number;
	reservedEur: number;
	feePct: number;
	inventory: Record<string, number>;
	openCount: number;
};

const emptyWallet = (): WalletView => ({
	currency: "EUR",
	balance: 0,
	reservedEur: 0,
	feePct: 0,
	inventory: {},
	openCount: 0,
});

type Listener = () => void;

/*
WalletDataProvider mirrors hub wallet snapshots for the header.
*/
class WalletDataProviderImpl {
	private latest = emptyWallet();
	private listeners = new Set<Listener>();

	subscribe(listener: Listener) {
		this.listeners.add(listener);

		return () => {
			this.listeners.delete(listener);
		};
	}

	snapshot(): WalletView {
		return this.latest;
	}

	private notify() {
		for (const listener of this.listeners) {
			listener();
		}
	}

	private toView(payload: WalletPayload): WalletView {
		const inventory = payload.Inventory ?? {};
		let openCount = 0;

		for (const qty of Object.values(inventory)) {
			if (qty > 0) {
				openCount++;
			}
		}

		return {
			currency: payload.Currency ?? "EUR",
			balance: payload.Balance ?? 0,
			reservedEur: payload.ReservedEUR ?? 0,
			feePct: payload.FeePct ?? 0,
			inventory,
			openCount,
		};
	}

	ingest(raw: unknown) {
		if (!isWalletPayload(raw)) {
			return;
		}

		this.latest = this.toView(raw);
		this.notify();
	}
}

const shared = new WalletDataProviderImpl();

export const WalletDataProvider = {
	subscribe: (listener: Listener) => shared.subscribe(listener),
	snapshot: () => shared.snapshot(),
	ingest: (raw: unknown) => shared.ingest(raw),
};

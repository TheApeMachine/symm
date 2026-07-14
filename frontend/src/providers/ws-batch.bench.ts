import { bench, describe } from "vitest";
import { FrameBatcher } from "./ws-batch";
import { applyFramePayload } from "./ws-stores";

const payload = {
	tick: {
		count: 12,
		open: 2,
		candidates: 4,
		quotes_ready: 8,
		quotes_total: 10,
	},
	positions: [
		{
			symbol: "BTC/USD",
			qty: 0.01,
			entry_price: 61000,
			mark: 61420,
			pnl: 4.2,
			return_pct: 0.0068852459,
		},
	],
	stops: {
		"BTC/USD": {
			symbol: "BTC/USD",
			stop_price: 61200,
			peak_return: 0.008,
			stop_return: 0.0032,
			momentum_health: 0.66,
			momentum_active: true,
			stagnation_health: 0.75,
			stagnation_pending: false,
			stagnation_active: true,
		},
	},
	balances: [
		{
			asset: "USD",
			balance: 200,
			available: 180,
			reserved: 20,
		},
	],
};

describe("ws batching", () => {
	bench("coalesces a burst of websocket payloads", () => {
		const batcher = new FrameBatcher(() => {});

		for (let index = 0; index < 32; index += 1) {
			batcher.enqueue({
				tick: { count: index, open: index % 3 },
				positions: payload.positions,
			});
		}

		batcher.dispose();
	});

	bench("applies one coalesced frame payload into stores", () => {
		applyFramePayload(payload);
	});
});

import { describe, expect, it } from "vitest";
import {
	applyFramePayload,
	frameStores,
	subscribe,
} from "#/providers/ws-stores";

const holding = {
	symbol: "BTC/USD",
	qty: 0.01,
	entry_price: 61000,
	entry_fee: 1.586,
	exit_fee: 1.597,
	mark: 61420,
	pnl: 4.2,
	return_pct: 0.0068852459,
};

describe("ws-stores", () => {
	it("skips stores on error frames", () => {
		frameStores.tick.actions.reset();

		const error = applyFramePayload({
			error: {
				level: "error",
				error: "get websockets token: EAPI:Invalid nonce",
				caller: "websocket/live.go:134",
			},
		});

		expect(error).toEqual({
			level: "error",
			error: "get websockets token: EAPI:Invalid nonce",
			caller: "websocket/live.go:134",
		});
		expect(frameStores.tick.state.version).toBe(0);
	});

	it("streams the circular buffer on subscribe", () => {
		frameStores.holdings.actions.reset();

		const seen: unknown[][] = [];
		const subscription = subscribe(
			frameStores.holdings,
			(state) => state.holdings["BTC/USD"],
			(rows) => {
				seen.push(rows);
			},
		);

		applyFramePayload({ holdings: [holding] });

		expect(seen).toHaveLength(1);
		expect(seen[0]?.[0]).toMatchObject({ symbol: "BTC/USD", qty: 0.01 });

		subscription.unsubscribe();
	});
});

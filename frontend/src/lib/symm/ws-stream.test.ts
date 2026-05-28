import { describe, expect, it } from "vitest";

import { OhlcDataProvider } from "#/components/symm/ohlc-data-provider";
import { routePayload } from "#/lib/symm/ws-stream";

describe("routePayload", () => {
	it("feeds mark events into trade chart history", () => {
		const bars: number[] = [];
		const unregister = OhlcDataProvider.registerSymbol("ROUTE/EUR", (bar) => {
			bars.push(bar.sec);
		});

		routePayload({
			event: "mark",
			ts: "2026-05-28T01:10:10Z",
			symbol: "ROUTE/EUR",
			price: 0.42,
		});

		expect(bars).toEqual([
			Math.floor(Date.parse("2026-05-28T01:10:10Z") / 1000),
		]);
		unregister();
	});
});

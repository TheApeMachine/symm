// @vitest-environment jsdom
import { act, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { focusAtom, RingBuffer, signals } from "#/collections/app";
import { Value } from "#/types/value";
import { CrossSectionPanel } from "./cross-section-panel";

describe("CrossSectionPanel", () => {
	it("renders panel and binds liquidity metrics live", () => {
		focusAtom.set("BTC/USD");
		const ring = new RingBuffer<Value>(10);
		signals.liquidity.setState((prev) => ({ ...prev, "BTC/USD": ring }));

		const view = render(<CrossSectionPanel />);

		expect(view.container.querySelector('[data-metric="symbol"]')?.textContent).toBe("—");

		act(() => {
			const val = new Value({
				scope: "BTC/USD",
				timestamp: 1_700_000_000_000_000_000n,
				metrics: [
					{ name: "relative_spread", raw: 0.0004, snm: 0.0004 },
					{ name: "best_bid_price", raw: 95123.45, snm: 95123.45 },
					{ name: "best_ask_price", raw: 95127.25, snm: 95127.25 },
				],
			});
			ring.add(val);
			signals.liquidity.setState((prev) => ({ ...prev, "BTC/USD": ring }));
		});

		expect(view.container.querySelector('[data-metric="symbol"]')?.textContent).toBe("BTC/USD");
		expect(view.container.querySelector('[data-metric-raw="best_bid_price"]')?.textContent).toBe("95123.45");
		expect(view.container.querySelector('[data-metric-raw="best_ask_price"]')?.textContent).toBe("95127.25");
		expect(view.container.querySelector('[data-metric-raw="relative_spread"]')?.textContent).toBe("4.0 bp");
	});
});

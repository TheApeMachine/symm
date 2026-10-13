// @vitest-environment jsdom
import { act, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { focusAtom, RingBuffer, signals } from "#/collections/app";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { CrossSectionPanel } from "./cross-section-panel";

describe("CrossSectionPanel", () => {
	it("renders panel and binds liquidity metrics live", () => {
		focusAtom.set("BTC/USD");
		const ring = new RingBuffer<MeasurementT>(10);
		signals.liquidity.setState((prev) => ({ ...prev, "BTC/USD": ring }));

		const view = render(<CrossSectionPanel />);

		expect(view.container.querySelector('[data-metric="symbol"]')?.textContent).toBe("—");

		act(() => {
			const m = new MeasurementT();
			m.symbol = "BTC/USD";
			m.at = 1_700_000_000_000_000_000n;
			m.metrics = [
				new MetricT("relative_spread", 0.0004, 0.0004),
				new MetricT("best_bid_price", 95123.45, 95123.45),
				new MetricT("best_ask_price", 95127.25, 95127.25),
			];
			ring.add(m);
			signals.liquidity.setState((prev) => ({ ...prev, "BTC/USD": ring }));
		});

		expect(view.container.querySelector('[data-metric="symbol"]')?.textContent).toBe("BTC/USD");
		expect(view.container.querySelector('[data-metric-raw="best_bid_price"]')?.textContent).toBe("95123.45");
		expect(view.container.querySelector('[data-metric-raw="best_ask_price"]')?.textContent).toBe("95127.25");
		expect(view.container.querySelector('[data-metric-raw="relative_spread"]')?.textContent).toBe("4.0 bp");
	});
});

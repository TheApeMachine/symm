// @vitest-environment jsdom
import { act, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { focusAtom, RingBuffer, signals } from "#/collections/app";
import { Value } from "#/types/value";
import { XrayFactsPanel } from "./xray-facts-panel";

describe("XrayFactsPanel", () => {
	it("renders panel and binds resonance metrics live", () => {
		focusAtom.set("BTC/USD");
		const ring = new RingBuffer<Value>(10);
		signals.resonance.setState((prev) => ({ ...prev, "BTC/USD": ring }));

		const view = render(<XrayFactsPanel />);

		// Initially fallback
		expect(view.container.querySelector('[data-metric="symbol"]')?.textContent).toBe("—");

		// Live value
		act(() => {
			const val = new Value({
				scope: "BTC/USD",
				metrics: [
					{ name: "energy", raw: 0.1234, snm: 0.1234 },
					{ name: "surprise", raw: 0.0567, snm: 0.0567 },
				],
				metadata: [{ name: "maturity", value: 42 }],
				provenance: [
					{ name: "confidence", value: "0.952" },
					{ name: "supported_horizon", value: "32" },
					{ name: "resolved_steps", value: "120" },
					{ name: "maturity", value: "42" },
				],
			});
			(val as any).maturity = 42;
			ring.add(val);
			signals.resonance.setState((prev) => ({ ...prev, "BTC/USD": ring }));
		});

		expect(view.container.querySelector('[data-metric="symbol"]')?.textContent).toBe("BTC/USD");
		expect(view.container.querySelector('[data-metric-raw="energy"]')?.textContent).toBe("0.1234");
		expect(view.container.querySelector('[data-metric-raw="surprise"]')?.textContent).toBe("0.0567");
		expect(view.container.querySelector('[data-metric="confidence"]')?.textContent).toBe("95.2%");
		expect(view.container.querySelector('[data-metric="supported_horizon"]')?.textContent).toBe("32");
		expect(view.container.querySelector('[data-metric="resolved_steps"]')?.textContent).toBe("120");
		expect(view.container.querySelector('[data-metric="maturity"]')?.textContent).toBe("42");
	});
});

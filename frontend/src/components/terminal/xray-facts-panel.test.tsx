// @vitest-environment jsdom
import { act, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { focusAtom, signals } from "#/collections/app";
import { Value } from "#/types/value";
import { XrayFactsPanel } from "./xray-facts-panel";

describe("XrayFactsPanel", () => {
	it("renders panel and binds resonance metrics live", () => {
		focusAtom.set("BTC/USD");
		signals.resonance.setState({});

		const view = render(<XrayFactsPanel />);

		// Initially fallback
		expect(
			view.container.querySelector('[data-metric="symbol"]')?.textContent,
		).toBe("—");

		// Live value
		act(() => {
			const prefix = "resonance/reading/BTC-USD";
			const enc = new TextEncoder();
			const layers: Record<string, number | string> = {
				energy: 0.1234,
				surprise: 0.0567,
				confidence: 0.952,
				supported_horizon: 32,
				resolved_steps: 120,
				maturity: 42,
			};
			const index = Object.keys(layers).map((key) => `${prefix}/${key}`);
			const val = new Value({
				scope: "BTC/USD",
				prefix,
				index,
				layers: Object.values(layers).map((v) =>
					enc.encode(JSON.stringify([v])),
				),
			});
			signals.resonance.setState((prev) => ({ ...prev, "BTC/USD": val }));
		});

		expect(
			view.container.querySelector('[data-metric="symbol"]')?.textContent,
		).toBe("BTC/USD");
		expect(
			view.container.querySelector('[data-metric-raw="energy"]')?.textContent,
		).toBe("0.1234");
		expect(
			view.container.querySelector('[data-metric-raw="surprise"]')?.textContent,
		).toBe("0.0567");
		expect(
			view.container.querySelector('[data-metric="confidence"]')?.textContent,
		).toBe("95.2%");
		expect(
			view.container.querySelector('[data-metric="supported_horizon"]')
				?.textContent,
		).toBe("32");
		expect(
			view.container.querySelector('[data-metric="resolved_steps"]')
				?.textContent,
		).toBe("120");
		expect(
			view.container.querySelector('[data-metric="maturity"]')?.textContent,
		).toBe("42");
	});
});

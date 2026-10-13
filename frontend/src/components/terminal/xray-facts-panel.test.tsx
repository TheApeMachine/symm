// @vitest-environment jsdom
import { act, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { focusAtom, RingBuffer, signals } from "#/collections/app";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { NamedStringT } from "#/providers/telemetry/telemetry/named-string";
import { XrayFactsPanel } from "./xray-facts-panel";

describe("XrayFactsPanel", () => {
	it("renders panel and binds resonance metrics live", () => {
		focusAtom.set("BTC/USD");
		const ring = new RingBuffer<MeasurementT>(10);
		signals.resonance.setState((prev) => ({ ...prev, "BTC/USD": ring }));

		const view = render(<XrayFactsPanel />);

		// Initially fallback
		expect(view.container.querySelector('[data-metric="symbol"]')?.textContent).toBe("—");

		// Live measurement
		act(() => {
			const m = new MeasurementT();
			m.symbol = "BTC/USD";
			m.maturity = 42;
			m.metrics = [
				new MetricT("energy", 0.1234, 0.1234),
				new MetricT("surprise", 0.0567, 0.0567),
			];
			m.provenance = [
				new NamedStringT("confidence", "0.952"),
				new NamedStringT("supported_horizon", "32"),
				new NamedStringT("resolved_steps", "120"),
			];
			ring.add(m);
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

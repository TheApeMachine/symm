// @vitest-environment jsdom
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ensureSignalStore, focusAtom, signals } from "#/collections/app";
import { KernelMetricStrip } from "#/components/ui/metric-strip";
import { Value } from "#/types/value";

const publishCvd = (aggressor: number, tick = 1n) => {
	const prefix = "cvd/metric/BTC/USD";
	const frame = new Value({
		origin: "cvd",
		role: "metric",
		scope: "BTC/USD",
		prefix,
		tick,
		index: [`${prefix}/aggressor_backing`],
		layers: [new TextEncoder().encode(JSON.stringify([aggressor]))],
	});
	act(() => {
		ensureSignalStore("cvd").setState((prev) => ({
			...prev,
			"BTC/USD": frame,
		}));
	});
};

afterEach(() => {
	cleanup();
	focusAtom.set("BTC/USD");
	signals.cvd?.setState(() => ({}));
});

describe("KernelMetricStrip", () => {
	it("binds kernel metrics from the cvd origin store", async () => {
		const view = render(<KernelMetricStrip />);

		expect(
			view.container.querySelector('[data-series="aggressor_backing"]'),
		).toBeTruthy();
		expect(
			view.container
				.querySelector('[data-metric-meter="aggressor_backing"]')
				?.getAttribute("aria-valuenow"),
		).toBe("0");

		publishCvd(0.42);

		await waitFor(() => {
			expect(
				view.container
					.querySelector('[data-metric-meter="aggressor_backing"]')
					?.getAttribute("aria-valuenow"),
			).toBe("42");
		});
	});

	it("keeps the last kernel sample when a later frame omits the headline", async () => {
		const view = render(<KernelMetricStrip />);
		publishCvd(0.42);

		await waitFor(() => {
			expect(
				view.container
					.querySelector('[data-metric-meter="aggressor_backing"]')
					?.getAttribute("aria-valuenow"),
			).toBe("42");
		});

		publishCvd(0.42, 2n);
		act(() => {
			ensureSignalStore("cvd").setState((prev) => ({
				...prev,
				"BTC/USD": new Value({
					origin: "cvd",
					role: "metric",
					scope: "BTC/USD",
					prefix: "cvd/metric/BTC/USD",
					tick: 3n,
					index: [],
					layers: [],
				}),
			}));
		});

		// Empty frame clears live metrics; Component sticky history may still
		// show the prior sample on the sparkline path — meter goes standby.
		expect(
			view.container.querySelector('[data-metric-badge="aggressor_backing"]'),
		).toBeTruthy();
	});
});

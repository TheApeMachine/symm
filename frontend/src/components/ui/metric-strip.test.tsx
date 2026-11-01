// @vitest-environment jsdom
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { focusAtom } from "#/collections/app";
import { ingestEnvelope, stores } from "#/collections/envelope";
import { KernelMetricStrip } from "#/components/ui/metric-strip";

const publishCvd = (aggressor: number | null) => {
	act(() => {
		ingestEnvelope({
			cvd: {
				"BTC/USD":
					aggressor === null
						? {}
						: { aggressor_backing: aggressor },
			},
		});
	});
};

afterEach(() => {
	cleanup();
	focusAtom.set("BTC/USD");
	stores.cvd?.setState(() => ({}));
});

describe("KernelMetricStrip", () => {
	it("binds kernel metrics from the cvd envelope store", async () => {
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

	it("clears the live meter when the headline leaves the facet", async () => {
		const view = render(<KernelMetricStrip />);
		publishCvd(0.42);

		await waitFor(() => {
			expect(
				view.container
					.querySelector('[data-metric-meter="aggressor_backing"]')
					?.getAttribute("aria-valuenow"),
			).toBe("42");
		});

		publishCvd(null);

		await waitFor(() => {
			expect(
				view.container.querySelector(
					'[data-metric-badge="aggressor_backing"]',
				)?.textContent,
			).toMatch(/Standby/i);
		});
	});
});

// @vitest-environment jsdom
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { focusAtom, signals } from "#/collections/app";
import { KernelMetricStrip } from "#/components/ui/metric-strip";
import { Value } from "#/types/value";

const publishCvd = (signedNet: number) => {
	const prefix = "workspace/tick/aggregate/0/1";
	const aggregate = new Value({
		origin: "workspace",
		role: "tick",
		scope: "aggregate",
		prefix,
		tick: 1n,
		index: [`${prefix}/cvd/BTC/USD/signed_net_fraction`],
		layers: [new TextEncoder().encode(JSON.stringify([signedNet]))],
	});
	act(() => {
		signals.workspace.setState((prev) => ({ ...prev, aggregate }));
	});
};

afterEach(() => {
	cleanup();
	focusAtom.set("BTC/USD");
	signals.workspace.setState(() => ({}));
});

describe("KernelMetricStrip", () => {
	it("binds folded kernel metrics off the workspace aggregate tick", async () => {
		const view = render(<KernelMetricStrip />);
		const cvdRow = view.container.querySelector('[data-series="signed_net_fraction"]');

		expect(cvdRow).toBeTruthy();
		expect(
			view.container.querySelector('[data-metric-meter="signed_net_fraction"]')
				?.getAttribute("aria-valuenow"),
		).toBe("0");

		publishCvd(0.42);

		const cvdMeter = () =>
			view.container.querySelector('[data-metric-meter="signed_net_fraction"]');

		await waitFor(() => {
			expect(cvdMeter()?.getAttribute("aria-valuenow")).toBe("42");
		});
	});

	it("keeps the last kernel sample when a tick has no reading", async () => {
		const view = render(<KernelMetricStrip />);
		publishCvd(0.42);

		const cvdMeter = () =>
			view.container.querySelector('[data-metric-meter="signed_net_fraction"]');

		await waitFor(() => {
			expect(cvdMeter()?.getAttribute("aria-valuenow")).toBe("42");
		});

		act(() => {
			signals.workspace.setState((prev) => ({
				...prev,
				aggregate: new Value({
					origin: "workspace",
					role: "tick",
					scope: "aggregate",
					prefix: "workspace/tick/aggregate/0/2",
					tick: 2n,
					index: [],
					layers: [],
				}),
			}));
		});

		expect(cvdMeter()?.getAttribute("aria-valuenow")).toBe("42");
		expect(
			view.container.querySelector('[data-metric-badge="signed_net_fraction"]')
				?.textContent,
		).not.toBe("STANDBY");
	});
});

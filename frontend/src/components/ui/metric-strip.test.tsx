// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
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
	it("binds folded kernel metrics off the workspace aggregate tick", () => {
		const view = render(<KernelMetricStrip />);
		const cvdRow = view.container.querySelector('[data-series="signed_net_fraction"]');

		expect(cvdRow).toBeTruthy();
		expect(
			view.container.querySelector('[data-metric-raw="signed_net_fraction"]')
				?.textContent,
		).toBe("—");

		publishCvd(0.42);

		expect(
			view.container.querySelector('[data-metric-raw="signed_net_fraction"]')
				?.textContent,
		).toBe("0.420");
	});
});

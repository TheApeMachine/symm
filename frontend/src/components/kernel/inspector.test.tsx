// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_FOCUS_SYMBOL, signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { KernelInspector } from "#/components/kernel/inspector";
import { Value } from "#/types/value";

vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => () => {},
}));

const renderInspector = () => renderToStaticMarkup(<KernelInspector />);

const publishKernel = (
	kernel: string,
	symbol: string,
	metrics: Record<string, number>,
	tick: bigint,
) => {
	const prefix = `workspace/tick/aggregate/0/${tick}`;
	const keys = Object.keys(metrics);
	const aggregate = new Value({
		origin: "workspace",
		role: "tick",
		scope: "aggregate",
		prefix,
		tick,
		index: keys.map((key) => `${prefix}/${kernel}/${symbol}/${key}`),
		layers: keys.map((key) =>
			new TextEncoder().encode(JSON.stringify([metrics[key]])),
		),
	});
	act(() => {
		signals.workspace.setState((prev) => ({ ...prev, aggregate }));
	});
};

describe("KernelInspector", () => {
	afterEach(() => {
		cleanup();
		signals.workspace.setState(() => ({}));
		signals.resonance.setState(() => ({}));
		terminalStore.actions.closeInspect();
	});

	it("renders nothing when no kernel is being inspected", () => {
		terminalStore.actions.closeInspect();
		expect(renderInspector()).toBe("");
	});

	it("renders the kernel's identity, blurb, history and meters", () => {
		publishKernel(
			"hawkes",
			DEFAULT_FOCUS_SYMBOL,
			{ branching_spectral_radius: 2.5 },
			1n,
		);
		terminalStore.actions.inspectSource("hawkes");

		const markup = renderInspector();

		expect(markup).toContain("Hawkes process");
		expect(markup).toContain("branching η");
		expect(markup).toContain("Healthy");
		expect(markup).toContain("Self-exciting point process");
		expect(markup).toContain("Signal history");
		expect(markup).toMatch(/<polyline/);
		expect(markup).toContain("Open in signal insight");
	});

	it("renders a meter for every metric the kernel publishes", () => {
		publishKernel(
			"toxicity",
			DEFAULT_FOCUS_SYMBOL,
			{ "retreat_fraction:bid": 1.25 },
			1n,
		);
		terminalStore.actions.inspectSource("toxicity");

		const markup = renderInspector();

		expect(markup).toContain("Signal metrics");
		expect(markup).toContain("retreat fraction · bid");
		expect(markup).toContain("1.2500");

		terminalStore.actions.inspectSource("resonance");
		const resonanceMarkup = renderInspector();
		expect(resonanceMarkup).toContain("predictive confidence");
		expect(resonanceMarkup).not.toContain("Signal metrics");
	});

	it("formats sub-basis-point values using scientific notation", () => {
		publishKernel(
			"toxicity",
			DEFAULT_FOCUS_SYMBOL,
			{ "retreat_fraction:bid": 0.0000117 },
			1n,
		);
		terminalStore.actions.inspectSource("toxicity");

		expect(renderInspector()).toContain("1.17e-5");
	});

	it("reports a kernel with no readings as standby rather than crashing", () => {
		terminalStore.actions.inspectSource("toxicity");

		const markup = renderInspector();

		expect(markup).toContain("Toxicity");
		expect(markup).toContain("Standby");
		expect(markup).toContain("no readings yet");
	});

	it("repaints the history sparkline when the workspace tick advances", () => {
		publishKernel(
			"hawkes",
			DEFAULT_FOCUS_SYMBOL,
			{ branching_spectral_radius: 0.2 },
			1n,
		);
		terminalStore.actions.inspectSource("hawkes");

		const view = render(<KernelInspector />);
		const spark = () =>
			view.container.querySelector("svg polyline[fill='none']");
		const before = spark()?.getAttribute("points");

		act(() => {
			publishKernel(
				"hawkes",
				DEFAULT_FOCUS_SYMBOL,
				{ branching_spectral_radius: 0.9 },
				2n,
			);
		});

		const after = spark()?.getAttribute("points");
		expect(after).not.toBe(before);
	});

	it("opens on resonance, which publishes no headline metric", () => {
		terminalStore.actions.inspectSource("resonance");

		const markup = renderInspector();

		expect(markup).toContain("Resonance");
		expect(markup).toContain("predictive confidence");
		expect(markup).toContain("Confidence");
	});
});

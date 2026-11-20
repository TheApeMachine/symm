// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_FOCUS_SYMBOL, signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { KernelInspector } from "#/components/kernel/inspector";

vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => () => {},
}));

const renderInspector = () => renderToStaticMarkup(<KernelInspector />);

const publishKernel = (
	kernel: string,
	symbol: string,
	metrics: Record<string, number>,
) => {
	const store = signals[kernel];
	if (!store) {
		throw new Error(`unknown kernel store: ${kernel}`);
	}
	act(() => {
		store.setState((prev) => ({ ...prev, [symbol]: metrics }));
	});
};

describe("KernelInspector", () => {
	afterEach(() => {
		cleanup();
		for (const kernel of Object.keys(signals)) {
			signals[kernel]?.setState(() => ({}));
		}
		terminalStore.actions.closeInspect();
	});

	it("renders nothing when no kernel is being inspected", () => {
		terminalStore.actions.closeInspect();
		expect(renderInspector()).toBe("");
	});

	it("renders the kernel identity and latest reading", () => {
		publishKernel("hawkes", DEFAULT_FOCUS_SYMBOL, { spectral_radius: 2.5 });
		terminalStore.actions.inspectSource("hawkes");

		const markup = renderInspector();

		expect(markup).toContain("Hawkes process");
		expect(markup).toContain("Self-exciting point process");
		expect(markup).toContain("Latest reading");
		expect(markup).toMatch(/<polyline/);
		expect(markup).toContain("Open in signal insight");
	});

	it("renders a meter for every metric the kernel publishes", () => {
		publishKernel("toxicity", DEFAULT_FOCUS_SYMBOL, {
			support_score: 1.25,
		});
		terminalStore.actions.inspectSource("toxicity");

		const markup = renderInspector();

		expect(markup).toContain("Signal metrics");
		expect(markup).toContain("support score");
		expect(markup).toContain("1.2500");

		terminalStore.actions.inspectSource("resonance");
		const resonanceMarkup = renderInspector();
		expect(resonanceMarkup).toContain("predictive confidence");
		expect(resonanceMarkup).not.toContain("Signal metrics");
	});

	it("reports a kernel with no readings as standby", () => {
		terminalStore.actions.inspectSource("toxicity");

		const markup = renderInspector();

		expect(markup).toContain("Toxicity");
		expect(markup).toContain("awaiting first reading");
	});

	it("updates when the signal store changes", () => {
		publishKernel("hawkes", DEFAULT_FOCUS_SYMBOL, { spectral_radius: 0.2 });
		terminalStore.actions.inspectSource("hawkes");

		const view = render(<KernelInspector />);
		const level = () =>
			view.container.querySelector('[role="progressbar"]')?.textContent;

		expect(level()).toContain("0.20");

		act(() => {
			publishKernel("hawkes", DEFAULT_FOCUS_SYMBOL, {
				spectral_radius: 0.9,
			});
		});

		view.rerender(<KernelInspector />);
		expect(level()).toContain("0.90");
	});

	it("opens on resonance", () => {
		publishKernel("resonance", DEFAULT_FOCUS_SYMBOL, { confidence: 0.42 });
		terminalStore.actions.inspectSource("resonance");

		const markup = renderInspector();

		expect(markup).toContain("Resonance");
		expect(markup).toContain("predictive confidence");
		expect(markup).toContain("Confidence");
	});
});
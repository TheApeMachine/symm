import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { terminalStore } from "#/collections/terminal";

/*
The inspector calls useNavigate purely for the footer action. Stubbing it keeps
these tests on the panel's own rendering — what it shows for a given kernel and
its readings — rather than standing up a router that renders nothing here.
*/
vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => () => {},
}));

const { getKernelReadingStore } = await import("#/collections/app");
const { KernelInspector } = await import("#/components/kernel/inspector");

const renderInspector = () => renderToStaticMarkup(<KernelInspector />);

describe("KernelInspector", () => {
	it("renders nothing when no kernel is being inspected", () => {
		terminalStore.actions.closeInspect();

		expect(renderInspector()).toBe("");
	});

	it("renders the kernel's identity, blurb, history and meters", () => {
		getKernelReadingStore("hawkes").state.clear();
		getKernelReadingStore("hawkes").actions.add(2.5);
		terminalStore.actions.inspectSource("hawkes");

		const markup = renderInspector();

		// Identity: name, sub-label and a status badge.
		expect(markup).toContain("Hawkes process");
		expect(markup).toContain("branching η");
		expect(markup).toContain("Healthy");

		// The blurb the mockup leads the body with.
		expect(markup).toContain("Self-exciting point process");

		// A real history section with a drawn trace, not an empty label.
		expect(markup).toContain("Signal history");
		expect(markup).toMatch(/<polyline/);
		expect(markup).toContain("1 reading");

		// Meters and the footer action.
		expect(markup).toContain("History");
		expect(markup).toContain("Open in signal insight");

		terminalStore.actions.closeInspect();
	});

	it("reports a kernel with no readings as standby rather than crashing", () => {
		getKernelReadingStore("toxicity").state.clear();
		terminalStore.actions.inspectSource("toxicity");

		const markup = renderInspector();

		expect(markup).toContain("Toxicity");
		expect(markup).toContain("Standby");
		expect(markup).toContain("no readings yet");
		expect(markup).toContain("awaiting first reading");

		terminalStore.actions.closeInspect();
	});

	/*
		Resonance is not a measurement source and has no headline metric, so the
		panel must name its own quantity. Looking one up threw, which crashed the
		modal into a blank overlay for that one row.
	*/
	it("opens on resonance, which publishes no headline metric", () => {
		terminalStore.actions.inspectSource("resonance");

		const markup = renderInspector();

		expect(markup).toContain("Resonance");
		expect(markup).toContain("predictive confidence");
		expect(markup).toContain("Confidence");

		terminalStore.actions.closeInspect();
	});
});

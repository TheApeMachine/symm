import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { RingBuffer } from "#/collections/ring";
import { terminalStore } from "#/collections/terminal";
import type { WireMeasurement } from "#/types/capnp/measurement";

/*
The inspector calls useNavigate purely for the footer action. Stubbing it keeps
these tests on the panel's own rendering — what it shows for a given kernel and
its readings — rather than standing up a router that renders nothing here.
*/
vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => () => {},
}));

const { DEFAULT_FOCUS_SYMBOL, signals } = (await import(
	"#/collections/app"
)) as any;
const { KernelInspector } = await import("#/components/kernel/inspector");

const renderInspector = () => renderToStaticMarkup(<KernelInspector />);

const metricMeasurement = (
	snr: number,
	key: string,
	raw: number,
	normalized: number,
): WireMeasurement => ({
	id: "m-1",
	source: "hawkes",
	symbol: DEFAULT_FOCUS_SYMBOL,
	tick: 1n,
	at: 1000n,
	timestamp: 1000n,
	entity: 1,
	snr,
	maturity: 0.8,
	separation: 0.1,
	metrics: [{ name: key, raw, normalized }],
	metadata: {},
	provenance: [],
});

const sparseMeasurement = (snr: number): WireMeasurement => ({
	id: "m-sparse",
	source: "hawkes",
	symbol: DEFAULT_FOCUS_SYMBOL,
	tick: 2n,
	at: 2000n,
	timestamp: 2000n,
	entity: 1,
	snr,
	maturity: 0.8,
	separation: 0.1,
	metrics: [],
	metadata: {},
	provenance: [],
});

const setSignalRing = (source: string, measurements: WireMeasurement[]) => {
	const ring = new RingBuffer<WireMeasurement>(50);
	for (const m of measurements) ring.add(m);
	signals[source].setState({
		[DEFAULT_FOCUS_SYMBOL]: ring,
		"": ring,
	});
};

const clearSignalRing = (source: string) => {
	signals[source].setState({});
};

describe("KernelInspector", () => {
	it("renders nothing when no kernel is being inspected", () => {
		terminalStore.actions.closeInspect();

		expect(renderInspector()).toBe("");
	});

	it("renders the kernel's identity, blurb, history and meters", () => {
		clearSignalRing("hawkes");
		setSignalRing("hawkes", [sparseMeasurement(2.5)]);
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

	it("renders a meter for every metric the kernel publishes", () => {
		clearSignalRing("toxicity");
		setSignalRing("toxicity", [
			metricMeasurement(3.5, "retreat_rate", 1.25, 0.5),
		]);
		terminalStore.actions.inspectSource("toxicity");

		const markup = renderInspector();

		expect(markup).toContain("Signal metrics");

		// A known toxicity metric is named and its raw readout is shown, while
		// metrics the row does not carry stay dashed rather than zero.
		expect(markup).toContain("retreat rate");
		expect(markup).toContain("1.2500");
		expect(markup).toMatch(/1 \/ \d+ read/);

		// Resonance still names its own quantity and no metric grid.
		terminalStore.actions.inspectSource("resonance");
		const resonanceMarkup = renderInspector();
		expect(resonanceMarkup).toContain("predictive confidence");
		expect(resonanceMarkup).not.toContain("Signal metrics");

		terminalStore.actions.closeInspect();
	});

	it("holds a metric's last value across rows that do not carry it", () => {
		clearSignalRing("toxicity");
		setSignalRing("toxicity", [
			metricMeasurement(3.5, "retreat_rate", 1.25, 0.5),
			sparseMeasurement(2.0),
		]);
		terminalStore.actions.inspectSource("toxicity");

		const markup = renderInspector();

		// The readout keeps 1.25 instead of blinking back to a dash.
		expect(markup).toContain("retreat rate");
		expect(markup).toContain("1.2500");

		terminalStore.actions.closeInspect();
	});

	it("reports a kernel with no readings as standby rather than crashing", () => {
		clearSignalRing("toxicity");
		terminalStore.actions.inspectSource("toxicity");

		const markup = renderInspector();

		expect(markup).toContain("Toxicity");
		expect(markup).toContain("Standby");
		expect(markup).toContain("no readings yet");
		expect(markup).toContain("awaiting first reading");

		terminalStore.actions.closeInspect();
	});

	it("opens on resonance, which publishes no headline metric", () => {
		terminalStore.actions.inspectSource("resonance");

		const markup = renderInspector();

		expect(markup).toContain("Resonance");
		expect(markup).toContain("predictive confidence");
		expect(markup).toContain("Confidence");

		terminalStore.actions.closeInspect();
	});
});

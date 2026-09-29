import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { DEFAULT_FOCUS_SYMBOL, RingBuffer, signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { KernelInspector } from "#/components/kernel/inspector";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";

/*
The inspector calls useNavigate purely for the footer action. Stubbing it keeps
these tests on the panel's own rendering — what it shows for a given kernel and
its readings — rather than standing up a router that renders nothing here.
*/
vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => () => {},
}));

const renderInspector = () => renderToStaticMarkup(<KernelInspector />);

const metricMeasurement = (
	snr: number,
	key: string,
	raw: number,
	normalized: number,
): MeasurementT => {
	const m = new MeasurementT();
	m.source = "hawkes";
	m.symbol = DEFAULT_FOCUS_SYMBOL;
	m.snr = snr;
	m.snrDefined = true;
	const metric = new MetricT();
	metric.name = key;
	metric.raw = raw;
	metric.normalized = normalized;
	metric.hasNormalized = true;
	m.metrics = [metric];
	return m;
};

const sparseMeasurement = (snr: number): MeasurementT => {
	const m = new MeasurementT();
	m.source = "hawkes";
	m.symbol = DEFAULT_FOCUS_SYMBOL;
	m.snr = snr;
	m.snrDefined = true;
	m.metrics = [];
	return m;
};

const getTestRing = (source: string, symbol: string) => {
	const store = signals[source];
	if (!store) throw new Error(`unknown source ${source}`);
	let ring = store.state[symbol];
	if (!ring) {
		ring = new RingBuffer<MeasurementT>(50);
		store.state[symbol] = ring;
	}
	return ring;
};

describe("KernelInspector", () => {
	it("renders nothing when no kernel is being inspected", () => {
		terminalStore.actions.closeInspect();

		expect(renderInspector()).toBe("");
	});

	it("renders the kernel's identity, blurb, history and meters", () => {
		const ring = getTestRing("hawkes", DEFAULT_FOCUS_SYMBOL);
		ring.clear();
		ring.add(sparseMeasurement(2.5));
		signals.hawkes.setState((prev) => ({ ...prev }));
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
		const ring = getTestRing("toxicity", DEFAULT_FOCUS_SYMBOL);
		ring.clear();
		ring.add(metricMeasurement(3.5, "retreat_rate", 1.25, 0.5));
		signals.toxicity.setState((prev) => ({ ...prev }));
		terminalStore.actions.inspectSource("toxicity");

		const markup = renderInspector();

		expect(markup).toContain("Signal metrics");

		// A known toxicity metric is named and its raw readout is shown, while
		// metrics the row does not carry stay dashed rather than zero.
		expect(markup).toContain("retreat rate");
		expect(markup).toContain("1.2500");
		expect(markup).toMatch(/2 \/ \d+ read/);

		// Resonance still names its own quantity and no metric grid.
		terminalStore.actions.inspectSource("resonance");
		const resonanceMarkup = renderInspector();
		expect(resonanceMarkup).toContain("predictive confidence");
		expect(resonanceMarkup).not.toContain("Signal metrics");

		terminalStore.actions.closeInspect();
	});

	it("holds a metric's last value across rows that do not carry it", () => {
		const ring = getTestRing("toxicity", DEFAULT_FOCUS_SYMBOL);
		ring.clear();
		ring.add(metricMeasurement(3.5, "retreat_rate", 1.25, 0.5));
		ring.add(sparseMeasurement(2.0));
		signals.toxicity.setState((prev) => ({ ...prev }));
		terminalStore.actions.inspectSource("toxicity");

		const markup = renderInspector();

		expect(markup).toContain("retreat rate");
		expect(markup).toContain("1.2500");

		terminalStore.actions.closeInspect();
	});

	it("reports a kernel with no readings as standby rather than crashing", () => {
		const ring = getTestRing("toxicity", DEFAULT_FOCUS_SYMBOL);
		ring.clear();
		signals.toxicity.setState((prev) => ({ ...prev }));
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

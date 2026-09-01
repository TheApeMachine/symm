import { renderToStaticMarkup } from "react-dom/server";
import * as flatbuffers from "flatbuffers";
import { describe, expect, it, vi } from "vitest";
import { terminalStore } from "#/collections/terminal";
import { EnvelopeMeasurement } from "#/providers/telemetry/telemetry/envelope-measurement";
import { EnvelopeMeasurementMetric } from "#/providers/telemetry/telemetry/envelope-measurement-metric";
import { EnvelopeMetric } from "#/providers/telemetry/telemetry/envelope-metric";

/*
The inspector calls useNavigate purely for the footer action. Stubbing it keeps
these tests on the panel's own rendering — what it shows for a given kernel and
its readings — rather than standing up a router that renders nothing here.
*/
vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => () => {},
}));

const { getKernelReadingStore, getMeasurementStore } = await import(
	"#/collections/app"
);
const { KernelInspector } = await import("#/components/kernel/inspector");

const renderInspector = () => renderToStaticMarkup(<KernelInspector />);

/*
metricMeasurement builds a real EnvelopeMeasurement row naming a single metric
with raw and normalized values, exactly as the wire serializes one, so the
inspector's metric grid reads the same data the live dispatcher delivers.
*/
const metricMeasurement = (
	snr: number,
	key: string,
	raw: number,
	normalized: number,
): EnvelopeMeasurement => {
	const builder = new flatbuffers.Builder(0);
	const keyOffset = builder.createString(key);
	const labelOffset = builder.createString(key);

	EnvelopeMetric.startEnvelopeMetric(builder);
	EnvelopeMetric.addLabel(builder, labelOffset);
	EnvelopeMetric.addRaw(builder, raw);
	EnvelopeMetric.addNormalized(builder, normalized);
	EnvelopeMetric.addHasNormalized(builder, true);
	const metricValue = EnvelopeMetric.endEnvelopeMetric(builder);

	EnvelopeMeasurementMetric.startEnvelopeMeasurementMetric(builder);
	EnvelopeMeasurementMetric.addKey(builder, keyOffset);
	EnvelopeMeasurementMetric.addValue(builder, metricValue);
	const metric = EnvelopeMeasurementMetric.endEnvelopeMeasurementMetric(
		builder,
	);

	const metrics = EnvelopeMeasurement.createMetricsVector(builder, [metric]);

	EnvelopeMeasurement.startEnvelopeMeasurement(builder);
	EnvelopeMeasurement.addSnr(builder, snr);
	EnvelopeMeasurement.addSnrDefined(builder, true);
	EnvelopeMeasurement.addMetrics(builder, metrics);
	const offset = EnvelopeMeasurement.endEnvelopeMeasurement(builder);

	builder.finish(offset);

	return EnvelopeMeasurement.getRootAsEnvelopeMeasurement(
		new flatbuffers.ByteBuffer(builder.asUint8Array()),
	);
};

/*
sparseMeasurement builds a real EnvelopeMeasurement row carrying no metrics at
all — exactly a backend update that omits a measurement's vocabulary for that
tick. It exercises the grid's hold-last-value behavior: a sparse row landing
after a populated one must not flicker the readout back to a dash.
*/
const sparseMeasurement = (snr: number): EnvelopeMeasurement => {
	const builder = new flatbuffers.Builder(0);

	EnvelopeMeasurement.startEnvelopeMeasurement(builder);
	EnvelopeMeasurement.addSnr(builder, snr);
	EnvelopeMeasurement.addSnrDefined(builder, true);
	const offset = EnvelopeMeasurement.endEnvelopeMeasurement(builder);

	builder.finish(offset);

	return EnvelopeMeasurement.getRootAsEnvelopeMeasurement(
		new flatbuffers.ByteBuffer(builder.asUint8Array()),
	);
};

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

	/*
		The dashboard modal must show every metric a kernel publishes, not just
		its level and history. Resonance is a presentation surface with no
		measurement vocabulary and is covered by its own dedicated case below, so
		this reads a real measurement row for a known source and asserts the grid
		carries that kernel's metric names with their current readouts.
	*/
	it("renders a meter for every metric the kernel publishes", () => {
		getKernelReadingStore("toxicity").state.clear();
		getMeasurementStore("toxicity").state.clear();
		getMeasurementStore("toxicity").actions.add(
			metricMeasurement(3.5, "retreat_rate", 1.25, 0.5),
		);
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

	/*
		Backend rows are sparse: a metric can be absent from the newest update
		while still carrying a real, current value in a slightly older row. The
		readout must hold the last value it received rather than flicker to a
		dash for the ticks between X-bearing rows.
	*/
	it("holds a metric's last value across rows that do not carry it", () => {
		getKernelReadingStore("toxicity").state.clear();
		getMeasurementStore("toxicity").state.clear();
		// First, X is published with a value.
		getMeasurementStore("toxicity").actions.add(
			metricMeasurement(3.5, "retreat_rate", 1.25, 0.5),
		);
		// Then a sparse row with no metrics lands on top of it.
		getMeasurementStore("toxicity").actions.add(sparseMeasurement(2.0));
		terminalStore.actions.inspectSource("toxicity");

		const markup = renderInspector();

		// The readout keeps 1.25 instead of blinking back to a dash.
		expect(markup).toContain("retreat rate");
		expect(markup).toContain("1.2500");

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

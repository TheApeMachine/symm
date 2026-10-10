// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { focusAtom, positionCountAtom, RingBuffer, signals } from "#/collections/app";
import { LearningDevelopmentT } from "#/providers/telemetry/telemetry/learning-development";
import { LearningQuantityT } from "#/providers/telemetry/telemetry/learning-quantity";
import { LearningRegionT } from "#/providers/telemetry/telemetry/learning-region";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { NamedStringT } from "#/providers/telemetry/telemetry/named-string";
import { LearningDashboard } from "./dashboard";

afterEach(() => {
	cleanup();
	signals.training.setState(() => ({}));
	positionCountAtom.set(0);
});

describe("LearningDashboard", () => {
	it("renders received coordinates and basin IDs instead of grouping sources", () => {
		focusAtom.set("BTC/USD");
		const measurement = new MeasurementT();
		measurement.source = "training";
		measurement.symbol = "BTC/USD";
		measurement.grid = new LearningDevelopmentT();
		measurement.grid.symbol = "BTC/USD";
		const left = new LearningQuantityT();
		Object.assign(left, {
			id: 101n,
			source: "same-owner",
			label: "left",
			x: -1,
			y: 1,
			activity: 2,
			present: true,
		});
		const right = new LearningQuantityT();
		Object.assign(right, {
			id: 202n,
			source: "same-owner",
			label: "right",
			x: 1,
			y: -1,
			activity: 1,
			present: true,
		});
		const basin = new LearningRegionT();
		Object.assign(basin, { id: 101n, strength: 3, authority: 0.8, members: 2 });
		measurement.grid.quantities = [left, right];
		measurement.grid.regions = [basin];
		const ring = new RingBuffer<MeasurementT>(4);
		ring.add(measurement);
		signals.training.setState(() => ({ "BTC/USD": ring }));
		const { container } = render(<LearningDashboard />);
		expect(
			container.querySelector('[data-l="map-meta"]')?.textContent,
		).toContain("2 numeric cells · 1 hot regions");
		const circles = container.querySelectorAll('[data-l="map-points"] circle');
		expect(circles).toHaveLength(2);
		expect(circles[0].getAttribute("cx")).not.toEqual(
			circles[1].getAttribute("cx"),
		);
		const previous = circles[0].getAttribute("cx");
		left.x = 0;
		act(() => {
			ring.add(measurement);
			signals.training.setState((state) => ({ ...state }));
		});
		expect(circles[0].getAttribute("cx")).not.toEqual(previous);
	});

	it("renders actual backend data for training stage and blocker", () => {
		focusAtom.set("BTC/USD");
		const measurement = new MeasurementT();
		measurement.source = "training";
		measurement.symbol = "BTC/USD";
		measurement.provenance = [
			new NamedStringT("stage", "HISTORICAL VALIDATION"),
			new NamedStringT("stage_blocker", "uncertainty spans zero (need lower_bound > 0)"),
		];
		measurement.metrics = [
			new MetricT("stage_code", 1),
		];

		const ring = new RingBuffer<MeasurementT>(4);
		ring.add(measurement);
		signals.training.setState(() => ({ "BTC/USD": ring }));

		const { container } = render(<LearningDashboard />);
		const stageEl = container.querySelector('[data-l="training-stage"]');
		const blockerEl = container.querySelector('[data-l="stage-blocker"]');

		expect(stageEl?.textContent).toBe("HISTORICAL VALIDATION");
		expect(blockerEl?.textContent).toBe("uncertainty spans zero (need lower_bound > 0)");
	});

	it("renders replay progress, fragment counts, temporal precursor, and ABC boundaries", () => {
		focusAtom.set("BTC/USD");
		const measurement = new MeasurementT();
		measurement.source = "training";
		measurement.symbol = "BTC/USD";
		measurement.metrics = [
			new MetricT("stage_code", 0),
			new MetricT("steps", 1234),
			new MetricT("decisions", 56),
			new MetricT("resolved", 42),
			new MetricT("fragments_up", 15),
			new MetricT("fragments_up_friction", 4),
			new MetricT("fragments_down", 12),
			new MetricT("fragments_chop", 8),
			new MetricT("fragments_flat", 7),
			new MetricT("fragments_unsupported", 2),
			new MetricT("precursor_length", 4),
			new MetricT("mark_a", 100),
			new MetricT("mark_b", 120),
			new MetricT("mark_c", 150),
		];

		const ring = new RingBuffer<MeasurementT>(4);
		ring.add(measurement);
		signals.training.setState(() => ({ "BTC/USD": ring }));

		const { container } = render(<LearningDashboard />);

		expect(container.querySelector('[data-metric="steps"]')?.textContent).toBe("1,234");
		expect(container.querySelector('[data-metric="decisions"]')?.textContent).toBe("56");
		expect(container.querySelector('[data-metric="fragments_up"]')?.textContent).toBe("15");
		expect(container.querySelector('[data-metric="fragments_up_friction"]')?.textContent).toBe("4");
		expect(container.querySelector('[data-metric="fragments_down"]')?.textContent).toBe("12");
		expect(container.querySelector('[data-metric="fragments_chop"]')?.textContent).toBe("8");
		expect(container.querySelector('[data-metric="fragments_flat"]')?.textContent).toBe("7");
		expect(container.querySelector('[data-metric="fragments_unsupported"]')?.textContent).toBe("2");

		// Temporal Precursor sequence: renders "unavailable" when token identities are absent
		const precEl = container.querySelector('[data-l="temporal-precursor"]');
		expect(precEl?.textContent).toContain("unavailable");

		// ABC Boundaries
		const abcEl = container.querySelector('[data-l="abc-markers"]');
		expect(abcEl?.textContent).toContain("A: 100");
		expect(abcEl?.textContent).toContain("B: 120");
		expect(abcEl?.textContent).toContain("C: 150");
	});

	it("renders actual token hashes when precursor_tokens are published", () => {
		focusAtom.set("BTC/USD");
		const measurement = new MeasurementT();
		measurement.source = "training";
		measurement.symbol = "BTC/USD";
		measurement.provenance = [
			new NamedStringT("precursor_tokens", "0x1234,0x5678"),
		];
		measurement.metrics = [
			new MetricT("stage_code", 0),
			new MetricT("precursor_length", 2),
		];

		const ring = new RingBuffer<MeasurementT>(4);
		ring.add(measurement);
		signals.training.setState(() => ({ "BTC/USD": ring }));

		const { container } = render(<LearningDashboard />);
		const precEl = container.querySelector('[data-l="temporal-precursor"]');
		expect(precEl?.textContent).toContain("[0x1234] → [0x5678]");
	});

	it("renders frozen predicted action, actual delayed label, and trie support", () => {
		focusAtom.set("BTC/USD");
		const measurement = new MeasurementT();
		measurement.source = "training";
		measurement.symbol = "BTC/USD";
		measurement.metrics = [
			new MetricT("action", 1), // ActionEnter
			new MetricT("excursion_type", 1), // UP
			new MetricT("support", 75),
			new MetricT("confidence", 0.88),
		];

		const ring = new RingBuffer<MeasurementT>(4);
		ring.add(measurement);
		signals.training.setState(() => ({ "BTC/USD": ring }));

		const { container } = render(<LearningDashboard />);

		expect(container.querySelector('[data-l="frozen-prediction"]')?.textContent).toBe("ENTER");
		expect(container.querySelector('[data-l="delayed-label"]')?.textContent).toBe("UP");
		expect(container.querySelector('[data-metric="support"]')?.textContent).toBe("75");
		expect(container.querySelector('[data-metric="confidence"]')?.textContent).toBe("88.0%");
	});

	it("renders historical held-out metrics and renders unavailable economic measurements as unavailable, never 0", () => {
		focusAtom.set("BTC/USD");
		const measurement = new MeasurementT();
		measurement.source = "training";
		measurement.symbol = "BTC/USD";
		// Initial state: 0 opportunities, undefined economic statistics
		measurement.metrics = [
			new MetricT("hist_opportunities", 0),
			new MetricT("hist_mean_return", 0),
			new MetricT("hist_lower_bound", 0),
		];

		const ring = new RingBuffer<MeasurementT>(4);
		ring.add(measurement);
		signals.training.setState(() => ({ "BTC/USD": ring }));

		const { container } = render(<LearningDashboard />);

		// Rule 40 & 52: Unavailable economic return MUST render as "—", NOT "0" or "0.0 bp"
		const histReturnEl = container.querySelector('[data-metric="hist_mean_return"]');
		const histLowerEl = container.querySelector('[data-metric="hist_lower_bound"]');
		expect(histReturnEl?.textContent).toBe("—");
		expect(histLowerEl?.textContent).toBe("—");

		// Now update with real demonstrated held-out metrics
		const updateMeasurement = new MeasurementT();
		updateMeasurement.source = "training";
		updateMeasurement.symbol = "BTC/USD";
		updateMeasurement.metrics = [
			new MetricT("hist_opportunities", 15),
			new MetricT("hist_correct_enter", 10),
			new MetricT("hist_missed_enter", 3),
			new MetricT("hist_false_enter", 2),
			new MetricT("hist_mean_return", 0.0025),
			new MetricT("hist_lower_bound", 0.0010),
		];

		act(() => {
			ring.add(updateMeasurement);
			signals.training.setState((s) => ({ ...s }));
		});

		expect(container.querySelector('[data-metric="hist_opportunities"]')?.textContent).toBe("15");
		expect(container.querySelector('[data-metric="hist_correct_enter"]')?.textContent).toBe("10");
		expect(container.querySelector('[data-metric="hist_missed_enter"]')?.textContent).toBe("3");
		expect(container.querySelector('[data-metric="hist_false_enter"]')?.textContent).toBe("2");
		expect(container.querySelector('[data-metric="hist_mean_return"]')?.textContent).toBe("25.0 bp");
		expect(container.querySelector('[data-metric="hist_lower_bound"]')?.textContent).toBe("10.0 bp");

		// Abstentions do not create a measured return, even with opportunities.
		const zeroMean = new MeasurementT();
		zeroMean.source = "training";
		zeroMean.symbol = "BTC/USD";
		zeroMean.metrics = [
			new MetricT("hist_opportunities", 8),
			new MetricT("hist_missed_enter", 8),
			new MetricT("hist_mean_return", 0),
			new MetricT("hist_lower_bound", 0),
		];
		act(() => {
			ring.add(zeroMean);
			signals.training.setState((s) => ({ ...s }));
		});
		expect(container.querySelector('[data-metric="hist_mean_return"]')?.textContent).toBe("—");
		expect(container.querySelector('[data-metric="hist_lower_bound"]')?.textContent).toBe("—");

		// A resolved break-even prediction has a real zero return.
		const resolvedZero = new MeasurementT();
		resolvedZero.source = "training";
		resolvedZero.symbol = "BTC/USD";
		resolvedZero.metrics = [...zeroMean.metrics, new MetricT("hist_false_enter", 1)];
		act(() => {
			ring.add(resolvedZero);
			signals.training.setState((state) => ({ ...state }));
		});
		expect(container.querySelector('[data-metric="hist_mean_return"]')?.textContent).toBe("0.0 bp");
		expect(container.querySelector('[data-metric="hist_lower_bound"]')?.textContent).toBe("—");
	});

	it("opens on the position life-cycle view in place of the old model-training view", () => {
		const { container } = render(<LearningDashboard />);

		expect(container.textContent).toContain("Position life-cycle");
		expect(container.textContent).not.toContain("Model training");
		expect(container.querySelector('[data-l="lifecycle-performance"]')).not.toBeNull();
		expect(container.querySelector('[data-l="tape-title"]')).toBeNull();
	});
});

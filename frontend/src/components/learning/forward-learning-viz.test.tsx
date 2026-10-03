// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { RingBuffer, signals } from "#/collections/app";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { NamedStringT } from "#/providers/telemetry/telemetry/named-string";
import { NamedNumberT } from "#/providers/telemetry/telemetry/named-number";
import { ForwardLearningViz } from "./forward-learning-viz";

afterEach(() => {
	cleanup();
	signals.training.setState(() => ({}));
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

it("starts playback when a completed episode arrives after an empty queue", async () => {
	vi.useFakeTimers();
	vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
		ok: true,
		json: async () => ({ branches: [] }),
	}));
	const { container } = render(<ForwardLearningViz />);
	expect(container.textContent).toContain("Awaiting historical replay");
	const ring = new RingBuffer<MeasurementT>(8);

	await act(async () => {
		for (const [index, price] of [100, 102, 101].entries()) {
			const frame = new MeasurementT();
			frame.source = "training";
			frame.symbol = "BTC/USD";
			frame.tick = BigInt(index + 1);
			frame.metrics = [new MetricT("price", price), new MetricT("stage_code", 1)];
			frame.metadata = [new NamedNumberT("excursion_start", 1)];
			if (index === 2) frame.provenance = [new NamedStringT("excursion_event", "completed")];
			ring.add(frame);
		}
		signals.training.setState(() => ({ "BTC/USD": ring }));
	});
	expect(container.textContent).toContain("1 frames evaluated");
	await act(async () => { vi.advanceTimersByTime(16); });
	expect(container.textContent).toContain("3 frames evaluated");
});

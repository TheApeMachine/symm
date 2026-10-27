import { describe, expect, it } from "vitest";
import {
	type HawkesTraceSample,
	hawkesTrace,
	orderTracePoints,
	samplesForTrace,
} from "./xray-hawkes-trace";

const arrival = (at: bigint, intensity = 0.2): HawkesTraceSample => ({
	at,
	intensity,
	postArrival: intensity + 0.5,
	baseline: 0.2,
	decay: 1,
});

describe("hawkesTrace", () => {
	it("keeps the jump vertical and decays using that event's fit", () => {
		const { points } = hawkesTrace(
			[arrival(0n), arrival(1_000_000_000n, 0.38)],
			10,
		);
		expect(points.slice(0, 2)).toEqual([
			{ at: 0n, intensity: 0.2 },
			{ at: 0n, intensity: 0.7 },
		]);
		expect(
			points.find((point) => point.at === 500_000_000n)?.intensity,
		).toBeCloseTo(0.2 + 0.5 * Math.exp(-0.5));
		expect(points.at(-1)?.intensity).toBeCloseTo(0.88);
	});

	it("advances and relaxes in quiet market time without inventing arrivals", () => {
		const samples = [arrival(0n), arrival(1_000_000_000n)];
		const initial = hawkesTrace(samples, 10, 1_000_000_000n);
		const later = hawkesTrace(samples, 10, 1_500_000_000n);
		expect(later.from - initial.from).toBe(500_000_000n);
		expect(later.through - later.from).toBe(initial.through - initial.from);
		expect(later.points.at(-1)?.intensity).toBeCloseTo(
			0.2 + 0.5 * Math.exp(-0.5),
		);
		expect(later.points.at(-1)?.at).toBe(1_500_000_000n);
	});

	it("retains separate same-time arrivals and supports a single fitted event", () => {
		const first = arrival(1_000_000_000n);
		const second = arrival(first.at, first.postArrival);
		const coincident = hawkesTrace([first, second], 10);
		expect(coincident.points.map((point) => point.intensity)).toEqual([
			0.2, 0.7, 0.7, 1.2,
		]);
		const single = hawkesTrace([first], 10, 2_000_000_000n);
		expect(single.points.at(-1)?.intensity).toBeCloseTo(0.2 + 0.5 / Math.E);
	});

	it("does not apply a later fit retroactively and clips expired events", () => {
		const samples = [
			arrival(0n),
			{ ...arrival(1_000_000_000n), baseline: 0.4, decay: 2 },
		];
		const plot = hawkesTrace(samples, 10, 2_000_000_000n);
		expect(plot.points.every((point) => point.at >= plot.from)).toBe(true);
		expect(plot.points.at(-1)?.intensity).toBeCloseTo(0.4 + 0.3 * Math.exp(-2));
	});

	it("linearly bridges ticks when the rest rate would sit above the peak", () => {
		const high: HawkesTraceSample = {
			at: 0n,
			intensity: 0.4,
			postArrival: 0.4,
			baseline: 0.9,
			decay: 1,
		};
		const low: HawkesTraceSample = {
			at: 1_000_000_000n,
			intensity: 0.25,
			postArrival: 0.25,
			baseline: 0.9,
			decay: 1,
		};
		const { points } = hawkesTrace([high, low], 20);
		const mids = points.filter(
			(point) => point.at > 0n && point.at < 1_000_000_000n,
		);
		expect(mids.length).toBeGreaterThan(0);
		expect(mids.every((point) => point.intensity <= high.postArrival)).toBe(
			true,
		);
		expect(mids.every((point) => point.intensity >= low.intensity)).toBe(true);
	});

	it("meets the next arrival at its pre-jump level instead of overshooting decay", () => {
		const first: HawkesTraceSample = {
			at: 0n,
			intensity: 0.2,
			postArrival: 1,
			baseline: 0.2,
			decay: 1,
		};
		const second: HawkesTraceSample = {
			at: 1_000_000_000n,
			intensity: 0.35,
			postArrival: 0.35,
			baseline: 0.2,
			decay: 1,
		};
		const extrapolated =
			first.baseline +
			(first.postArrival - first.baseline) * Math.exp(-1);
		expect(extrapolated).toBeGreaterThan(second.intensity);

		const { points } = hawkesTrace([first, second], 40);
		const atArrival = points.filter((point) => point.at === 1_000_000_000n);
		expect(atArrival.some((point) => point.intensity === second.intensity)).toBe(
			true,
		);
		expect(atArrival.every((point) => point.intensity <= extrapolated)).toBe(
			true,
		);
	});

	it("collapses aggregate ticks that share one frame timestamp", () => {
		const stacked = Array.from({ length: 40 }, (_, index) =>
			arrival(BigInt(index), 0.2 + index * 0.001),
		);
		expect(samplesForTrace(stacked)).toHaveLength(1);
		const lastSample = stacked.at(-1);
		expect(lastSample).toBeDefined();
		if (!lastSample) return;
		expect(samplesForTrace(stacked)[0]?.postArrival).toBeCloseTo(
			lastSample.postArrival,
		);
	});

	it("orders trace points in time for a monotonic fill path", () => {
		const { points } = hawkesTrace([arrival(0n), arrival(1_000_000_000n)], 40);
		const ordered = orderTracePoints(points);
		for (let index = 1; index < ordered.length; index += 1) {
			expect(ordered[index]?.at ?? 0n).toBeGreaterThanOrEqual(
				ordered[index - 1]?.at ?? 0n,
			);
		}
	});

	it("scrolls events off-screen when viewportSpan is supplied", () => {
		const samples = [
			arrival(0n),
			arrival(5_000_000_000n),
			arrival(20_000_000_000n),
		];
		const viewportSpan = 15_000_000_000n;
		const plot = hawkesTrace(samples, 100, 20_000_000_000n, viewportSpan);
		expect(plot.through).toBe(20_000_000_000n);
		expect(plot.from).toBe(5_000_000_000n);
		expect(plot.through - plot.from).toBe(viewportSpan);
		expect(plot.points.every((point) => point.at >= plot.from)).toBe(true);
	});
});

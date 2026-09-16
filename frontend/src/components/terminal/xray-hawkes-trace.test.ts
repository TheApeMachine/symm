import { describe, expect, it } from "vitest";
import { type HawkesTraceSample, hawkesTrace } from "./xray-hawkes-trace";

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
});

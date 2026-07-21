import { describe, expect, it } from "vitest";
import {
	aggregateFluidParticles,
	type TerminalFluidParticle,
} from "./fluid-particles";

const particle = (
	cellX: number,
	cellZ: number,
	phase: number,
): TerminalFluidParticle => ({
	source: "manifold",
	role: "particle",
	cellX,
	cellY: 0,
	cellZ,
	phase,
	omega: 1,
	amplitude: 1,
	heat: 0,
	velX: 0,
	velY: 0,
	velZ: 0,
	speed: 0,
});

describe("aggregateFluidParticles", () => {
	it("retains every focused oscillator as X–Z occupancy", () => {
		const particles = Array.from({ length: 12 }, (_, index) =>
			particle(index % 4, Math.floor(index / 4), 0),
		);

		const cells = aggregateFluidParticles(particles, 4, 3);

		expect(cells).toHaveLength(12);
		expect(cells.reduce((total, cell) => total + cell.count, 0)).toBe(12);
	});

	it("aggregates coincident particles and preserves phase cancellation", () => {
		const cells = aggregateFluidParticles(
			[particle(1, 2, 0), particle(1, 2, Math.PI)],
			4,
			4,
		);

		expect(cells).toHaveLength(1);
		expect(cells[0].count).toBe(2);
		expect(cells[0].amplitude).toBeCloseTo(0);
	});

	it("rejects particles outside the published projection", () => {
		const cells = aggregateFluidParticles(
			[particle(3.99, 0, 0), particle(8, 0, 0), particle(0, -1, 0)],
			4,
			4,
		);

		expect(cells).toHaveLength(1);
		expect(cells[0].column).toBe(3);
		expect(cells[0].count).toBe(1);
	});
});

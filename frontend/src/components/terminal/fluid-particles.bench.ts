import { bench, describe } from "vitest";
import {
	aggregateFluidParticles,
	type TerminalFluidParticle,
} from "./fluid-particles";

const particles: TerminalFluidParticle[] = Array.from(
	{ length: 4096 },
	(_, index) => ({
		source: "manifold",
		role: "particle",
		cellX: index % 64,
		cellY: (index * 11) % 64,
		cellZ: Math.floor(index / 64),
		phase: (index / 4096) * Math.PI * 2,
		omega: 1,
		amplitude: 1,
		heat: 0,
		velX: 0,
		velY: 0,
		velZ: 0,
		speed: 0,
	}),
);

describe("fluid-particles", () => {
	bench("aggregates a full 64×64 focused population", () => {
		aggregateFluidParticles(particles, 64, 64);
	});
});

import { bench, describe } from "vitest";
import { sampleBilinearLattice, terminalFluidFieldStats } from "./fluid-field";

const matrix = Array.from({ length: 38 }, (_, row) =>
	Array.from({ length: 64 }, (_, column) => {
		const x = column / 63;
		const y = row / 37;
		let value = 0;

		for (const blob of [
			{ x: 0.42, y: 0.28, amplitude: 1, radius: 0.18 },
			{ x: 0.58, y: 0.72, amplitude: 0.82, radius: 0.16 },
		]) {
			const deltaX = x - blob.x;
			const deltaY = y - blob.y;

			value +=
				blob.amplitude *
				Math.exp(
					-(deltaX * deltaX + deltaY * deltaY) /
						(2 * blob.radius * blob.radius),
				);
		}

		value += 0.08 * Math.sin(x * 14) * Math.cos(y * 9);

		return Math.min(1, value / 1.6);
	}),
);

describe("fluid-field", () => {
	bench("derives raw occupancy stats for a 64x38 rho projection", () => {
		terminalFluidFieldStats(matrix);
	});

	bench(
		"bilinearly samples a 64x38 rho projection for a 640x380 canvas",
		() => {
			for (let row = 0; row < 380; row += 1) {
				const sampleY = row / 379;

				for (let column = 0; column < 640; column += 1) {
					sampleBilinearLattice(matrix, column / 639, sampleY);
				}
			}
		},
	);
});

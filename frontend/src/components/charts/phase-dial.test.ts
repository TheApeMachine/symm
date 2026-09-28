import { describe, expect, it } from "vitest";
import type { FluidOscillator } from "#/components/fluid-3d/wire";
import { paintPhaseDial, type PhaseDialState } from "./phase-dial";

describe("paintPhaseDial", () => {
	it("retains predigested channel resultants from the physics kernel", () => {
		const oscillators: FluidOscillator[] = [
			{ phase: 0, omega: -1, amplitude: 2, heat: 0.5, side: "bid" },
			{ phase: Math.PI, omega: -2, amplitude: 1, heat: 0.8, side: "bid" },
			{ phase: Math.PI / 2, omega: 1, amplitude: 3, heat: 0.2, side: "ask" },
		];

		const state: PhaseDialState = {
			oscillators,
			wave: [],
			status: { ready: true, reason: "" },
			resultants: [
				{
					side: "bid",
					count: 2,
					totalAmplitude: 3,
					coherence: 1 / 3,
					phase: 0,
				},
				{
					side: "ask",
					count: 1,
					totalAmplitude: 3,
					coherence: 1,
					phase: Math.PI / 2,
				},
			],
		};

		expect(() => paintPhaseDial(state)).not.toThrow();
	});
});

import { describe, expect, it } from "vitest";
import type { FluidOscillator } from "#/components/fluid-3d/wire";
import { phaseChannelResultants } from "./phase-dial";

describe("phaseChannelResultants", () => {
	it("keeps bid and ask phasors separate and amplitude weighted", () => {
		const oscillators: FluidOscillator[] = [
			{ phase: 0, omega: -1, amplitude: 2, side: "bid" },
			{ phase: Math.PI, omega: -2, amplitude: 1, side: "bid" },
			{ phase: Math.PI / 2, omega: 1, amplitude: 3, side: "ask" },
		];

		const [bid, ask] = phaseChannelResultants(oscillators);

		expect(bid).toEqual({
			side: "bid",
			count: 2,
			totalAmplitude: 3,
			coherence: expect.closeTo(1 / 3, 12),
			phase: expect.closeTo(0, 12),
		});
		expect(ask).toEqual({
			side: "ask",
			count: 1,
			totalAmplitude: 3,
			coherence: 1,
			phase: expect.closeTo(Math.PI / 2, 12),
		});
	});
});

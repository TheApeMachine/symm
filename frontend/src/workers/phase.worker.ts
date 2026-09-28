import * as Comlink from "comlink";
import type { PhaseChannelResultant } from "#/components/charts/phase-dial";
import type { FluidOscillator } from "#/components/fluid-3d/wire";

export type PhaseComputations = {
	resultants: PhaseChannelResultant[];
	kuramotoPsi: number;
	maxOmega: number;
	kuramotoOscillators: { phase: number; heat: number }[];
};

export const computePhaseSynchronous = (
	oscillators: FluidOscillator[],
): PhaseComputations => {
	let meanSin = 0;
	let meanCos = 0;
	let maxOmega = 0;

	let bidReal = 0;
	let bidImag = 0;
	let bidTotalAmp = 0;
	let bidCount = 0;

	let askReal = 0;
	let askImag = 0;
	let askTotalAmp = 0;
	let askCount = 0;

	for (const osc of oscillators) {
		const cosP = Math.cos(osc.phase);
		const sinP = Math.sin(osc.phase);

		meanSin += sinP;
		meanCos += cosP;
		const absOmega = Math.abs(osc.omega);
		if (absOmega > maxOmega) {
			maxOmega = absOmega;
		}

		if (osc.side === "bid") {
			bidReal += osc.amplitude * cosP;
			bidImag += osc.amplitude * sinP;
			bidTotalAmp += osc.amplitude;
			bidCount += 1;
		} else if (osc.side === "ask") {
			askReal += osc.amplitude * cosP;
			askImag += osc.amplitude * sinP;
			askTotalAmp += osc.amplitude;
			askCount += 1;
		}
	}

	const count = oscillators.length;
	const kuramotoPsi =
		count > 0 ? Math.atan2(meanSin / count, meanCos / count) : 0;

	const resultants: PhaseChannelResultant[] = [
		{
			side: "bid",
			count: bidCount,
			totalAmplitude: bidTotalAmp,
			coherence:
				bidTotalAmp > 0 ? Math.hypot(bidReal, bidImag) / bidTotalAmp : 0,
			phase: bidCount > 0 ? Math.atan2(bidImag, bidReal) : 0,
		},
		{
			side: "ask",
			count: askCount,
			totalAmplitude: askTotalAmp,
			coherence:
				askTotalAmp > 0 ? Math.hypot(askReal, askImag) / askTotalAmp : 0,
			phase: askCount > 0 ? Math.atan2(askImag, askReal) : 0,
		},
	];

	const kuramotoOscillators = oscillators.map((oscillator) => ({
		phase: oscillator.phase,
		heat:
			maxOmega > 0 ? Math.min(1, Math.abs(oscillator.omega) / maxOmega) : 0,
	}));

	return {
		resultants,
		kuramotoPsi,
		maxOmega,
		kuramotoOscillators,
	};
};

const api = {
	computePhase(oscillators: FluidOscillator[]): PhaseComputations {
		return computePhaseSynchronous(oscillators);
	},
};

export type PhaseWorkerApi = typeof api;

Comlink.expose(api);

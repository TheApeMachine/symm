import type { CausalFrame } from "#/collections/types";

const finite = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

/*
latestCausalFrame returns the newest causal outcome from a flat buffer snapshot.
When symbol is provided, the newest matching row wins; otherwise the buffer tail.
*/
export const latestCausalFrame = (
	frames: readonly CausalFrame[] | undefined,
	symbol?: string,
): CausalFrame | undefined => {
	if (frames === undefined || frames.length === 0) {
		return undefined;
	}

	if (symbol === undefined || symbol === "") {
		return frames.at(-1);
	}

	for (let index = frames.length - 1; index >= 0; index -= 1) {
		const frame = frames[index];

		if (frame?.symbol === symbol) {
			return frame;
		}
	}

	return frames.at(-1);
};

export const causalReading = (
	frame: CausalFrame | undefined,
): CausalFrame["reading"] => frame?.reading;

export const causalStrength = (frame: CausalFrame | undefined): number =>
	finite(causalReading(frame)?.strength) ?? 0;

export const causalEntryBaseline = (frame: CausalFrame | undefined): number =>
	finite(causalReading(frame)?.entryBaseline) ?? 0;

export const causalConfidence = (frame: CausalFrame | undefined): number =>
	finite(causalReading(frame)?.confidence) ?? 0;

export const causalAssociation = (frame: CausalFrame | undefined): number =>
	finite(causalReading(frame)?.association) ?? 0;

export const causalIntervention = (frame: CausalFrame | undefined): number =>
	finite(causalReading(frame)?.intervention) ?? 0;

export const causalCounterfactual = (frame: CausalFrame | undefined): number =>
	finite(causalReading(frame)?.counterfactual) ?? 0;

export const causalUplift = (frame: CausalFrame | undefined): number =>
	finite(causalReading(frame)?.uplift) ?? 0;

export const causalNoise = (frame: CausalFrame | undefined): number =>
	finite(causalReading(frame)?.noise) ?? 0;

export const causalContagion = (frame: CausalFrame | undefined): number =>
	finite(causalReading(frame)?.contagion) ?? 0;

export const causalCategory = (frame: CausalFrame | undefined): number =>
	finite(causalReading(frame)?.category) ?? 0;

export const causalRatio = (value: number): number =>
	Math.min(1, Math.max(0, value));

export const causalUnit = (value: number): number =>
	Math.max(0, Math.min(1, value));

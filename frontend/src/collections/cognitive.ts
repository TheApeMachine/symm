import { createStore } from "@tanstack/react-store";

export type CognitiveReading = {
	scope: string;
	sequence: string;
	regimePrefix: string;
	regimeCohort: number;
	ambiguous: boolean;
	sideline: boolean;
	entropyBits: number;
	entropyThreshold: number;
	classConfidence: number;
	contrastEvidence: number;
	lookaheadScore: number;
	lookaheadPaths: number;
	winnerClass: string;
	prewarmPaths: number | null;
	prewarmScore: number | null;
	updatedAt: number;
};

const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const finiteCount = (value: unknown): number =>
	Math.max(0, Math.floor(finiteNumber(value) ?? 0));

const stringValue = (value: unknown): string =>
	typeof value === "string" ? value.trim() : "";

export const parseCognitiveFrame = (
	raw: Record<string, unknown>,
): CognitiveReading | null => {
	const scope = stringValue(raw.scope);

	if (scope === "") {
		return null;
	}

	return {
		scope: scope,
		sequence: stringValue(raw.sequence),
		regimePrefix: stringValue(raw.regime_prefix),
		regimeCohort: finiteCount(raw.regime_cohort),
		ambiguous: raw.ambiguous === true,
		sideline: raw.sideline === true,
		entropyBits: finiteNumber(raw.entropy_bits) ?? 0,
		entropyThreshold: finiteNumber(raw.entropy_threshold) ?? 0,
		classConfidence: finiteNumber(raw.class_confidence) ?? 0,
		contrastEvidence: finiteNumber(raw.contrast_evidence) ?? 0,
		lookaheadScore: finiteNumber(raw.lookahead_score) ?? 0,
		lookaheadPaths: finiteCount(raw.lookahead_paths),
		winnerClass: stringValue(raw.winner_class),
		prewarmPaths: finiteNumber(raw.prewarm_paths),
		prewarmScore: finiteNumber(raw.prewarm_score),
		updatedAt: Date.now(),
	};
};

export const cognitiveStore = createStore(
	{
		readings: {} as Record<string, CognitiveReading>,
		selectedScope: "",
	},
	({ setState }) => ({
		updateReading: (reading: CognitiveReading) =>
			setState((prev) => {
				const readings = {
					...prev.readings,
					[reading.scope]: reading,
				};

				const selectedScope =
					prev.selectedScope === "" ? reading.scope : prev.selectedScope;

				return {
					...prev,
					readings: readings,
					selectedScope: selectedScope,
				};
			}),
		selectScope: (scope: string) =>
			setState((prev) => ({
				...prev,
				selectedScope: scope,
			})),
	}),
);

export const cognitiveScopes = (): string[] =>
	Object.keys(cognitiveStore.state.readings).sort();

export const selectedCognitiveReading = (): CognitiveReading | null => {
	const scope = cognitiveStore.state.selectedScope;

	if (scope === "") {
		return null;
	}

	return cognitiveStore.state.readings[scope] ?? null;
};

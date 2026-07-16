import { createStore } from "@tanstack/react-store";

/*
CognitiveReading mirrors the backend types.Cognition payload exported on each
Thesis tick. It carries the sensory prefix tree, beam paths, and basin posteriors
that Cortex renders from thesis.cognition frames.
*/
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
	prewarmPaths?: number | null;
	prewarmScore?: number | null;
	updatedAt: number;
	beamWidth?: number;
	maxHops?: number;
	nodeCount?: number;
	branches?: CognitiveBranch[];
	beams?: CognitiveBeam[];
	classes?: CognitiveClass[];
	remFrom?: string;
	remThrough?: string;
	remReplays?: number;
};

export type CognitiveBranch = {
	id: number;
	parentId: number;
	token: string;
	prefix: string;
	depth: number;
	probability: number;
	count: number;
};

export type CognitiveBeam = {
	sequence: string;
	score: number;
};

export type CognitiveClass = {
	name: string;
	probability: number;
};

type CognitionFrame = {
	symbol?: string;
	at?: string;
	sequence?: string;
	regimePrefix?: string;
	winner?: string;
	ready?: boolean;
	confidence?: number;
	contrast?: number;
	entropyBits?: number;
	entropyThreshold?: number;
	ambiguous?: boolean;
	cohort?: number;
	lookaheadScore?: number;
	lookaheadPaths?: number;
	beamWidth?: number;
	maxHops?: number;
	nodeCount?: number;
	branches?: CognitiveBranch[];
	beams?: CognitiveBeam[];
	classes?: CognitiveClass[];
	remFrom?: string;
	remThrough?: string;
	remReplays?: number;
};

const finite = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const stringField = (value: unknown, fallback = ""): string =>
	typeof value === "string" ? value : fallback;

/*
cognitiveReadingFromFrame maps one backend Cognition record into the Cortex store
shape without dropping tree visualization fields.
*/
export const cognitiveReadingFromFrame = (
	frame: CognitionFrame,
): CognitiveReading | null => {
	const scope = stringField(frame.symbol);

	if (scope === "") {
		return null;
	}

	const updatedAt = Date.parse(stringField(frame.at));

	return {
		scope,
		sequence: stringField(frame.sequence),
		regimePrefix: stringField(frame.regimePrefix),
		regimeCohort: finite(frame.cohort) ?? 0,
		ambiguous: frame.ambiguous === true,
		sideline: frame.ready === false,
		entropyBits: finite(frame.entropyBits) ?? 0,
		entropyThreshold: finite(frame.entropyThreshold) ?? 0,
		classConfidence: finite(frame.confidence) ?? 0,
		contrastEvidence: finite(frame.contrast) ?? 0,
		lookaheadScore: finite(frame.lookaheadScore) ?? 0,
		lookaheadPaths: finite(frame.lookaheadPaths) ?? 0,
		winnerClass: stringField(frame.winner),
		updatedAt: Number.isFinite(updatedAt) ? updatedAt : Date.now(),
		beamWidth: finite(frame.beamWidth) ?? undefined,
		maxHops: finite(frame.maxHops) ?? undefined,
		nodeCount: finite(frame.nodeCount) ?? undefined,
		branches: frame.branches,
		beams: frame.beams,
		classes: frame.classes,
		remFrom: frame.remFrom,
		remThrough: frame.remThrough,
		remReplays: finite(frame.remReplays) ?? undefined,
	};
};

const readingsFromFrame = (
	frame: unknown,
): Record<string, CognitiveReading> | null => {
	if (Array.isArray(frame)) {
		const readings: Record<string, CognitiveReading> = {};

		for (const entry of frame) {
			if (entry === null || typeof entry !== "object") {
				continue;
			}

			const reading = cognitiveReadingFromFrame(entry as CognitionFrame);

			if (reading === null) {
				continue;
			}

			readings[reading.scope] = reading;
		}

		return Object.keys(readings).length === 0 ? null : readings;
	}

	if (frame === null || typeof frame !== "object") {
		return null;
	}

	const record = frame as Record<string, unknown>;
	const nestedReadings = record.readings;

	if (nestedReadings !== undefined && nestedReadings !== null) {
		return readingsFromFrame(nestedReadings);
	}

	const directReading = cognitiveReadingFromFrame(record as CognitionFrame);

	if (directReading === null) {
		return null;
	}

	return { [directReading.scope]: directReading };
};

/*
cognitiveStore holds the latest per-symbol cognitive readings broadcast on each
Thesis tick under cognition. The Cortex surface renders the DMT sequence tree,
entropy gate, and posterior from these.
*/
export const cognitiveStore = createStore(
	{
		readings: {} as Record<string, CognitiveReading>,
		selectedScope: null as string | null,
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((prev) => {
				const readings = readingsFromFrame(frame);

				if (readings === null) {
					return prev;
				}

				return {
					...prev,
					readings: {
						...prev.readings,
						...readings,
					},
				};
			}),
		selectScope: (selectedScope: string) =>
			setState((prev) => ({ ...prev, selectedScope })),
	}),
);

/*
cognitiveScopes lists the symbols that currently have a cognitive reading in a
stable order.
*/
export const cognitiveScopes = (
	readings: Record<string, CognitiveReading>,
): string[] => Object.keys(readings).sort();

import { createStore } from "@tanstack/react-store";

/*
CognitiveReading mirrors the backend market.CognitiveReading: a per-symbol
cognitive summary derived from the tick's measurements (regime sequence, entropy
gate, winning class). Optional prewarm fields are reserved for lookahead.
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

/*
cognitiveStore holds the latest per-symbol cognitive readings broadcast by the
trader (role "cognitive"). The Cortex surface renders the DMT sequence tree,
entropy gate, and posterior from these.
*/
export const cognitiveStore = createStore(
	{
		readings: {} as Record<string, CognitiveReading>,
		selectedScope: null as string | null,
	},
	({ setState }) => ({
		updateFrame: (frame: Record<string, unknown>) =>
			setState((prev) => {
				const readings = frame.readings as
					| Record<string, CognitiveReading>
					| undefined;

					if (readings === undefined || readings === null) {
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
cognitiveScopes lists the symbols that currently have a cognitive reading, in
descending class-confidence order so the crispest regime leads.
*/
export const cognitiveScopes = (
	readings: Record<string, CognitiveReading>,
): string[] =>
	Object.values(readings)
		.slice()
		.sort((left, right) => right.classConfidence - left.classConfidence)
		.map((reading) => reading.scope);

export type { TerminalSurface } from "#/collections/terminal";

/*
TerminalDecision mirrors a backend role=decision artifact one-to-one. The trader
has already decided; the frontend renders these raw fields directly and never
re-scores, re-ranks, or recomputes an edge.
*/
export type TerminalDecision = {
	symbol: string;
	side?: string;
	type?: string;
	price?: number;
	quantity?: number;
	confidence?: number;
	verdict: "allow" | "blocked";
	why: string;
	observed_at?: number;
	seq?: number;
};

export type TerminalKernel = {
	source: string;
	name?: string;
	category?: string;
	status?: string;
	statusLabel?: string;
	strengthText?: string;
	confidencePercent: number;
	surprisePercent: number;
	healthPercent?: number;
	confidenceText?: string;
	surpriseText?: string;
	samplesText?: string;
	activeText?: string;
	observedText?: string;
	faultText?: string;
};

export type TerminalDecisionRow = {
	key: string;
	symbol: string;
	source: string;
	scoreText: string;
	scoreValue: number;
	scoreMissing?: boolean;
	verdict: "allow" | "in-play" | "blocked";
	why: string;
	signals: Array<{
		source: string;
		confidence: number;
	}>;
	edgeText: string;
	edgePositive: boolean;
	fraction?: number;
	tick?: number;
	seq?: number;
	recency?: number;
};

export type TerminalModel = {
	wallet?: {
		available: string;
		reserved: string;
	};
	decisions?: TerminalDecisionRow[];
};

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

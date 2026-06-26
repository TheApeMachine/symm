export type { TerminalSurface } from "#/collections/terminal";

/*
TerminalKernel is one signal's contribution for the focused symbol, derived from
a backend measurement artifact (origin → confidence/surprise). Percent fields are
0–100 so view code can size bars without re-scaling.
*/
export type TerminalKernel = {
	source: string;
	confidencePercent: number;
	surprisePercent: number;
	// Display fields the kernel list/health panels populate from the same
	// measurement. Optional: the decision/allocation math needs only the core
	// percent fields, so callers that just rank candidates can omit these.
	name?: string;
	category?: string;
	status?: string;
	statusLabel?: string;
	strengthText?: string;
	healthPercent?: number;
	confidenceText?: string;
	surpriseText?: string;
	samplesText?: string;
	activeText?: string;
	observedText?: string;
	faultText?: string;
};

/*
TerminalDecisionRow is one candidate's row in the Decision Tree surface: its
combined score, playbook verdict, the signals that drove it, and its edge versus
the derived entry line. Built from real walk traces + measurement kernels.
*/
export type TerminalDecisionRow = {
	key: string;
	symbol: string;
	source: string;
	scoreText: string;
	scoreValue: number;
	verdict: "allow" | "in-play" | "blocked";
	why: string;
	signals: Array<{ source: string; confidence: number }>;
	edgeText: string;
	edgePositive: boolean;
};

/*
TerminalModel is the view-model slice the Allocation surface consumes: the wallet
figures and the current candidate decision rows it sizes positions from.
*/
export type TerminalModel = {
	wallet: {
		available: string;
		reserved: string;
	};
	decisions: TerminalDecisionRow[];
};

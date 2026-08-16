export type ReasoningTier = "measurement" | "field" | "scm" | "decision";

export type ReasoningNode = {
	id: string;
	label: string;
	symbol?: string;
	tier: ReasoningTier;
	role?: string;
	source?: string;
	value: number;
	confidence: number;
	derived: boolean;
	metadata?: Record<string, unknown>;
};

export type ReasoningLink = {
	from: string;
	to: string;
	relation: string;
	weight: number;
	confidence: number;
	derived: boolean;
};

export type ReasoningTopology = {
	symbol?: string;
	ready: boolean;
	reason: string;
	observedRows: number;
	maximumHorizon: number;
	treatment: string;
	mediator: string;
	target: string;
	controls: string[];
	currentState: Record<string, number>;
	nodes: ReasoningNode[];
	links: ReasoningLink[];
};

export type ReasoningSearchNode = {
	action: number;
	actionName: string;
	depth: number;
	visits: number;
	effectiveVisits: number;
	observedReward: number;
	counterfactualReward: number;
	counterfactualMass: number;
	counterfactualPrecision: number;
	totalReward: number;
	meanReward: number;
	exploitation: number;
	exploration: number;
	causalExpectation: number;
	selectionScore: number;
	scmReady: boolean;
	scmReason?: string;
	selected: boolean;
	principal: boolean;
	state?: Record<string, number>;
	children?: ReasoningSearchNode[];
};

export type ReasoningPayload = {
	reasoning?: ReasoningTopology;
	search?: ReasoningSearchNode;
	root?: ReasoningSearchNode;
};

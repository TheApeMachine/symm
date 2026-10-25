import type * as d3 from "d3";

export interface TrieNodeData {
	id: string;
	prefix: string;
	token?: string;
	action?: string;
	/** Path book threshold. 0.50 at the start, lower is closer to paper, higher is closer to prune. */
	threshold?: number;
	probability?: number;
	stepProbability?: number;
	tokens?: string[];
	isEnd?: boolean;
	isTerminal?: boolean;
	children?: TrieNodeData[];
	_children?: TrieNodeData[];
}

export interface TreeLink {
	source: d3.HierarchyPointNode<TrieNodeData>;
	target: d3.HierarchyPointNode<TrieNodeData>;
}

export interface ImpulseNode {
	id: string;
	label: string;
	cluster: number;
	snr: number;
	activation: number;
	x?: number;
	y?: number;
	vx?: number;
	vy?: number;
	gridX?: number;
	gridY?: number;
	value?: number;
	present?: boolean;
}

export interface TrieBranch {
	hash: string;
	depth: number;
	visits: number;
	meanEdge: number;
	confidence: number;
	policy: "ENTER" | "EXIT" | "NOOP"; // NOOP is a friction leaf policy
}

export interface FeasibleAction {
	rank: number;
	action: string;
	prefix: string;
	probability: number;
	state: string;
}

export interface CognitionTreeResponse {
	keys?: string[];
	root: TrieNodeData | null;
	branches: TrieBranch[];
	feasible: FeasibleAction[];
}

export interface LearningEpisode {
	id: number;
	type: "UPWARD EXCURSION" | "DOWNWARD EXCURSION" | "STAGNATION";
	symbol: string;
	points: { x: number; y: number }[];
	marks: { A: number; B: number; C: number };
	agent: {
		entryIdx: number | null;
		exitIdx: number | null;
		reward: number;
	};
	magnitude: number;
}

export interface LearningActivityEntry {
	id: number;
	time: string;
	message: string;
	pnl: number;
	action: string;
}

export interface SymbolProgress {
	symbol: string;
	path: string;
	depth: number;
	tokens: string[];
	/* Stored key dirs Step currently matches for this symbol; undefined when unknown. */
	matched?: string[];
	/* Next tokens the energy gate refused. */
	rejected?: string[];
}

export interface SequiturRuleExport {
	id: number;
	name: string;
	count: number;
	depth: number;
	length: number;
	symbols: string[];
	terminals: string[];
	action?: string;
}

export interface SequiturGrammarResponse {
	symbol: string;
	rules: SequiturRuleExport[];
	start: string[];
}

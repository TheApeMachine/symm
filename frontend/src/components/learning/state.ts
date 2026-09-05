import { useEffect, useState } from "react";

export type Region = {
	id: number;
	strength: number;
	authority: number;
	members: number;
};
export type Point = {
	id: number;
	source: string;
	label: string;
	x: number;
	y: number;
	value: number;
	energy: number;
	authority: number;
	present: boolean;
};
export type Prior = {
	Depth?: number;
	Pending?: number;
	EvidenceAuthority?: number;
	Memory?: number;
	Samples: number;
	Defined: boolean;
	Mean: number;
	Variance: number;
	VarianceDefined: boolean;
	Support: number;
	Maturity: number;
	Authority: number;
};
export type Token = {
	token: number;
	source: string;
	label: string;
	strength: number;
	authority: number;
	members: number;
};
export type Candidate = {
	knowledge?: Knowledge;
	kind: string;
	power: number;
	reduce: boolean;
	selected: boolean;
	prior: Prior;
};
export type Influence = {
	token: number;
	source: string;
	label: string;
	action: string;
	prior: Prior;
};
export type ExecutionStatus = {
	refused?: number;
	lastRefusal?: string;
	submitted: number;
	unsupported: number;
	diverged: number;
	dropped: number;
	failed: number;
	queued: number;
	lastFailure?: string;
};
export type Skill = {
	mode: string;
	account: string;
	since: string;
	reason: string;
	samples: number;
	support: number;
	defined: boolean;
	varianceDefined: boolean;
	qualified: boolean;
	mean: number;
	variance: number;
	standardError: number;
	lowerBound: number;
	confidence: number;
	sigma: number;
	memory: number;
	promotions: number;
	demotions: number;
	wins: number;
	losses: number;
};
export type Wallet = {
	lane: number;
	mode: string;
	cash: string;
	quantity: string;
	fees: string;
	equity: number;
	profit: number;
	rate: number;
	at: string;
	complete: boolean;
	action: { kind: string; power: number; reduce: boolean };
	pending: boolean;
	issued: number;
	fills: number;
	resolved: number;
	unresolved: number;
	prior: Prior;
	episodes: number;
	realized: number;
	spent: number;
	exhausted: boolean;
};
export type MissedOpportunity = {
	symbol: string;
	kind: string;
	fromAt: string;
	toAt: string;
	excursion: number;
	observations: number;
	exposed: boolean;
	unreviewable: boolean;
};
export type ForwardReview = {
	reviewed: number;
	exposed?: number;
	unexposed?: number;
	captured: number;
	missed: number;
	unreviewable: number;
	at: string;
	recent: MissedOpportunity[] | null;
};
export type LearningView = {
	capital?: CapitalView;
	warmup?: {
		resolved: number;
		unconditioned: number;
		unpaired: number;
		portfolioUnavailable: number;
	};
	at: string;
	symbol: string;
	status: string;
	steps: number;
	decisions: number;
	resolved: number;
	gridVersion: number;
	columns: number;
	initialCapital: string;
	skill: Skill;
	authorizedMode?: string;
	realizationAllowed?: boolean;
	realizationReason?: string;
	dispatched: number;
	rejected: number;
	rejection?: string;
	execution: ExecutionStatus;
	hasExecution: boolean;
	forward: ForwardReview;
	horizonNs: number;
	horizonEpochs: number;
	epochMean: number;
	epochs: number;
	universe: { symbol: string; status: string; decisions: number }[] | null;
	regions: Region[] | null;
	points: Point[] | null;
	lanes: Wallet[] | null;
	impulse: Token[] | null;
	candidates: Candidate[] | null;
	influence: Influence[] | null;
};
export type LearningEvent = {
	targetUnit?: string;
	absoluteSkillTarget?: number;
	baselineRate?: number;
	scope?: string;
	candidateId?: string;
	portfolioId?: string;
	candidateResult?: CandidateOutcome;
	id: number;
	lane: number;
	mode: string;
	kind: string;
	at: string;
	action: string;
	power: number;
	reduce: boolean;
	quantity?: string;
	fee?: string;
	gross?: string;
	cash: string;
	inventory: string;
	authority: number;
	profit: number;
	complete: boolean;
	target?: number;
	episode: number;
	truncated?: boolean;
	horizonNs: number;
	authorized?: string;
	prior: Prior;
};

export const baseUrl = () => {
	if (import.meta.env.VITE_SYMM_WS_URL) {
		return import.meta.env.VITE_SYMM_WS_URL.replace(/^ws/, "http").replace(
			/\/ws$/,
			"",
		);
	}
	const host =
		window.location.hostname === "localhost"
			? "127.0.0.1"
			: window.location.hostname;
	return `${window.location.protocol}//${host}:8765`;
};

export const useLearning = (symbol: string) => {
	const [view, setView] = useState<LearningView | null>(null);
	const [events, setEvents] = useState<LearningEvent[]>([]);
	const [error, setError] = useState("");

	useEffect(() => {
		const controller = new AbortController();
		let timer: ReturnType<typeof setTimeout>;
		const update = async () => {
			try {
				const response = await fetch(
					`${baseUrl()}/learning?symbol=${encodeURIComponent(symbol)}`,
					{ signal: controller.signal },
				);
				if (!response.ok)
					throw new Error(
						`Learning state: ${response.status} ${await response.text()}`,
					);
				const next: LearningView = await response.json();
				const journal = await fetch(
					`${baseUrl()}/learning/events?symbol=${encodeURIComponent(next.symbol)}`,
					{ signal: controller.signal },
				);
				if (!journal.ok)
					throw new Error(
						`Learning journal: ${journal.status} ${await journal.text()}`,
					);
				setEvents(await journal.json());
				setView(next);
				setError("");
			} catch (err) {
				if (!controller.signal.aborted) setError(String(err));
			} finally {
				// One-second display refresh; the backend learns on its own incoming events.
				if (!controller.signal.aborted) timer = setTimeout(update, 1000);
			}
		};
		void update();
		return () => {
			controller.abort();
			clearTimeout(timer);
		};
	}, [symbol]);
	return { view, events, error };
};

export type AgentSkill = {
	authorizedMode?: string;
	realizationAllowed?: boolean;
	realizationReason?: string;
	skill: Skill;
	dispatched: number;
	decisions: number;
	resolved: number;
	symbols: number;
};

/*
useAgentSkill polls the small projection of the same coherent snapshot the
learning surface reads, so the top bar can never disagree with it. A failed
poll leaves the last reading in place rather than presenting a fabricated one.
*/
export const useAgentSkill = () => {
	const [state, setState] = useState<AgentSkill | null>(null);
	const [error, setError] = useState("");

	useEffect(() => {
		const controller = new AbortController();
		let timer: ReturnType<typeof setTimeout>;
		const update = async () => {
			try {
				const response = await fetch(`${baseUrl()}/learning/skill`, {
					signal: controller.signal,
				});
				if (!response.ok) throw new Error(`Agent skill: ${response.status}`);
				setState(await response.json());
				setError("");
			} catch (err) {
				if (!controller.signal.aborted) setError(String(err));
			} finally {
				if (!controller.signal.aborted) timer = setTimeout(update, 1000);
			}
		};
		void update();
		return () => {
			controller.abort();
			clearTimeout(timer);
		};
	}, []);
	return { state, error };
};

export type Knowledge = {
	scope: string;
	global: Prior;
	symbol: Prior;
	selected: Prior;
};
export type AccountMark = {
	at: string;
	version: number;
	equity: number;
	netFunding: number;
	hasFunding: boolean;
};
export type AccountLearning = {
	state: {
		mark: AccountMark;
		cash: string;
		actualCash: string;
		committed: string;
		positions: Record<string, string> | null;
		complete: boolean;
		reason?: string;
	};
	outcome: { totalReward: number; rate: number; hasRate: boolean };
	target: number;
	resolved: number;
	mfe: number;
	mae: number;
	timeToPositiveNs: number;
	timeToBreakevenNs: number;
	holdingNs: number;
	trajectory: AccountMark[] | null;
	pending: string;
};
export type EntryClaim = {
	id: string;
	decision: number;
	symbol: string;
	action: string;
	power: number;
	at: string;
	marketAt: string;
	gridVersion: number;
	context: number[];
	scope: string;
	global: Prior;
	symbolPrior: Prior;
	prior: Prior;
	authority: number;
	quantity: string;
	notional: string;
	reference: string;
	horizonNs: number;
	feeRate: string;
	state: string;
	current: boolean;
	ageNs: number;
};
export type CandidateOutcome = {
	id: string;
	state: string;
	at: string;
	portfolioId?: string;
	detail?: string;
};
export type CapitalView = {
	choice: { symbol: string; kind: string; power: number };
	prior: Prior;
	decisions: number;
	actual: AccountLearning;
	exploration: AccountLearning;
	candidates: EntryClaim[] | null;
	outcomes: CandidateOutcome[] | null;
	demand: string;
};

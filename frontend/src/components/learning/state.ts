import { useSelector } from "@tanstack/react-store";
import { useEffect, useMemo, useState } from "react";
import { focusStore, onlineStore } from "#/collections/app";
import { learningStore } from "#/collections/learning";
import type { LearningAgentT } from "#/providers/telemetry/telemetry/learning-agent";
import type { LearningPriorT } from "#/providers/telemetry/telemetry/learning-prior";
import type { LearningStateT } from "#/providers/telemetry/telemetry/learning-state";

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
	Provisional?: boolean;
	Depth?: number;
	ContextLength?: number;
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

export type Skill = {
	mode: string;
	account: string;
	since: string;
	reason: string;
	samples: number;
	support: number;
	defined: boolean;
	varianceDefined: boolean;
	mean: number;
	variance: number;
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
	at: string;
	action: { kind: string; power: number; reduce: boolean };
	issued: number;
	fills: number;
	resolved: number;
	unresolved: number;
	prior: Prior;
	realized: number;
};

export type ForwardReview = { trained: number; at: string };
export type DeskTrader = {
	id: number;
	decisions: number;
	fills: number;
	graded: number;
	observed: number;
	quality: number;
	wealth: number;
	open: number;
	holding: number;
};
export type DeskView = {
	traders: DeskTrader[];
	settled: number;
};
export type LearningView = {
	rehearsal?: LearningStateT["rehearsal"];
	agents: LearningAgentT[];
	restored: boolean;
	desk?: DeskView;
	warmup?: {
		resolved: number;
		unconditioned: number;
		unpaired: number;
		portfolioUnavailable: number;
		targetUnavailable?: number;
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
	rejection?: string;
	forward: ForwardReview;
	precursorDepth?: number;
	precursorHistory?: Token[][] | null;
	horizonNs?: number;
	horizonObservations?: number;
	horizonCapped?: boolean;
	roundTrip?: number;
	movement?: number;
	hasMovement?: boolean;
	epochMean?: number;
	epochs?: number;
	universe: {
		symbol: string;
		status: string;
		present: number;
		regions: number;
	}[] | null;
	regions: Region[] | null;
	points: Point[] | null;
	lanes: Wallet[] | null;
	impulse: Token[] | null;
	candidates: Candidate[] | null;
	influence: Influence[] | null;
};

export type LearningEvent = {
	horizonSource?: string;
	targetUnit?: string;
	absoluteSkillTarget?: number;
	baselineRate?: number;
	scope?: string;
	candidateId?: string;
	portfolioId?: string;
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
	target?: number;
	truncated?: boolean;
	horizonNs: number;
	authorized?: string;
	prior: Prior;
};

export const useLearning = (symbol: string) => {
	const online = useSelector(onlineStore, (state) => state === "ONLINE");
	useEffect(() => {
		if (symbol) focusStore.setState(() => symbol);
	}, [symbol]);
	const state = useSelector(learningStore, (state) => state);
	const view = useMemo(
		() => (state ? projectLearning(state, symbol) : null),
		[state, symbol],
	);
	const [events, setEvents] = useState<LearningEvent[]>([]);
	useEffect(() => {
		if (!state) return;
		setEvents((previous) => updateLearningEvents(previous, state, symbol));
	}, [state, symbol]);
	return {
		view,
		events,
		error: !online
			? "Learning connection offline"
			: state && !learningStatusHealthy(state.status)
				? String(state.status)
				: "",
	};
};

// Repeated telemetry snapshots must not append the same display event twice.
export const updateLearningEvents = (
	previous: LearningEvent[],
	state: LearningStateT,
	symbol: string,
): LearningEvent[] => {
	// Display history is bounded by the 200-point chart budget, not a learning horizon.
	const next = [...previous];
	for (const member of state.agents ?? []) {
		const at = date(state.atNs);
		const last = member.last;
		if (
			last &&
			(!symbol || String(last.symbol) === symbol) &&
			!next.some(
				(event) =>
					event.lane === member.id &&
					event.id === Number(state.steps) &&
					event.kind === "valued" &&
					event.at === at,
			)
		) {
			next.push({
				id: Number(state.steps),
				lane: member.id,
				mode: member.id === 0 ? "policy" : "virtual",
				kind: "valued",
				at,
				action: String(last.action?.kind ?? ""),
				power: last.action?.power ?? 0,
				reduce: last.action?.reduce ?? false,
				cash: String(member.cash),
				inventory: "",
				authority: 0,
				profit: Number(member.profit),
				horizonNs: 0,
				prior: prior(member.reading),
			});
		}
		const outcome = member.outcome;
		if (
			outcome &&
			!next.some(
				(event) =>
					event.lane === member.id &&
					event.id === Number(outcome.id) &&
					event.kind === "resolved",
			)
		) {
			next.push({
				id: Number(outcome.id),
				lane: member.id,
				mode: member.id === 0 ? "policy" : "virtual",
				kind: "resolved",
				at: date(outcome.throughNs),
				action: String(outcome.action?.kind ?? ""),
				power: outcome.action?.power ?? 0,
				reduce: outcome.action?.reduce ?? false,
				cash: String(member.cash),
				inventory: "",
				authority: 0,
				profit: Number(member.profit),
				target: outcome.tape,
				horizonNs: Number(outcome.throughNs - outcome.atNs),
				prior: prior(member.reading),
			});
		}
	}
	return next.slice(-200);
};

export const useAgentSkill = () => {
	const online = useSelector(onlineStore, (state) => state === "ONLINE");
	const data = useSelector(learningStore, (state) => state);
	const view = data ? projectLearning(data, "") : null;
	return {
		state: view
			? {
					skill: view.skill,
					dispatched: view.dispatched,
					decisions: view.decisions,
					resolved: view.resolved,
					symbols: view.universe?.length ?? 0,
					authorizedMode: online ? view.status : "offline",
					realizationReason:
						"One live simulated account; historical workers have no wallets",
				}
			: null,
		error: !online
			? "Learning connection offline"
			: data && !learningStatusHealthy(data.status)
				? String(data.status)
				: "",
	};
};

const learningStatusHealthy = (status: string) =>
	status === "learning" ||
	status === "reading the record" ||
	status === "no tape" ||
	status === "forming the impulse map" ||
	status === "recognising precursors";

const date = (ns: bigint) => new Date(Number(ns / 1000000n)).toISOString();
const prior = (reading: LearningPriorT | null): Prior => ({
	Defined: reading?.defined ?? false,
	Mean: reading?.mean ?? 0,
	Variance: reading?.variance ?? 0,
	VarianceDefined: reading?.varianceDefined ?? false,
	Samples: Number(reading?.samples ?? 0n),
	Support: reading?.support ?? 0,
	Provisional: reading?.provisional,
	Authority: reading?.authority ?? 0,
	Maturity: reading?.maturity ?? 0,
	EvidenceAuthority: reading?.evidenceAuthority,
	Depth: reading?.depth,
	ContextLength: reading?.contextLength,
	Pending: Number(reading?.pending ?? 0n),
	Memory: reading?.memory,
});

// Only field selection, unit conversion, and display grouping happen here.
export const projectLearning = (
	state: LearningStateT,
	symbol: string,
): LearningView => {
	const agents = state.agents ?? [];
	const markets = state.markets ?? [];
	const member = agents[0];
	const market =
		markets.find((row) => String(row.symbol) === symbol) ??
		markets.find((row) => row.quantities.length > 0) ??
		markets[0];
	const reading = member?.reading;
	const at = date(state.atNs);
	const points = (market?.quantities ?? []).map((quantity, index) => ({
		id: index + 1,
		source: String(quantity.source),
		label: String(quantity.label),
		x: quantity.x,
		y: quantity.y,
		value: quantity.value,
		energy: quantity.activity,
		authority: quantity.quality,
		present: quantity.present,
	}));
	const regions = (market?.regions ?? []).map((region) => ({
		id: Number(region.id),
		strength: region.strength,
		authority: region.authority,
		members: region.members,
	}));
	const fills = agents.reduce(
		(total, member) => total + Number(member.fills),
		0,
	);
	return {
		agents,
		rehearsal: state.rehearsal,
		restored: state.restored,
		at,
		symbol: String(market?.symbol ?? ""),
		status: String(state.status),
		steps: Number(state.steps),
		decisions: Number(state.decisions),
		resolved: Number(state.resolved),
		gridVersion: Number(state.steps),
		columns: points.length,
		initialCapital: String(member?.initial ?? ""),
		skill: {
			mode: "learning",
			account: "simulated",
			since: at,
			reason: String(state.status),
			samples: Number(reading?.samples ?? 0n),
			support: reading?.support ?? 0,
			defined: reading?.defined ?? false,
			varianceDefined: reading?.varianceDefined ?? false,
			mean: reading?.mean ?? 0,
			variance: reading?.variance ?? 0,
			wins: Number(member?.wins ?? 0n),
			losses: Number(member?.losses ?? 0n),
		},
		dispatched: fills,
		forward: { trained: Number(state.resolved), at },
		universe: markets.map((row) => ({
			symbol: String(row.symbol),
			status: String(row.status),
			present: (row.quantities ?? []).filter((quantity) => quantity.present)
				.length,
			regions: row.regions?.length ?? 0,
		})),
		points,
		regions,
		horizonNs: market ? Number(market.atNs - market.fromNs) : undefined,
		epochs: Number(market?.decisions ?? 0n),
		impulse: (market?.regions ?? []).map((region) => ({
			token: Number(region.condition),
			source: points[Number(region.id) - 1]?.source ?? "",
			label: points[Number(region.id) - 1]?.label ?? "",
			strength: region.strength,
			authority: region.authority,
			members: region.members,
		})),
		candidates:
			member?.last?.symbol !== undefined &&
			member.last.symbol === market?.symbol
				? (member.alternatives ?? []).map((choice) => ({
						kind: String(choice.kind),
						power: choice.power,
						reduce: choice.reduce,
						prior: prior(choice.prior),
						selected:
							choice.kind === member.last?.action?.kind &&
							choice.power === member.last?.action?.power &&
							choice.reduce === member.last?.action?.reduce,
					}))
				: [],
		influence:
			member?.last?.symbol !== undefined &&
			member.last.symbol === market?.symbol
				? (member.alternatives ?? []).map((choice) => ({
						token: choice.prior?.depth ?? 0,
						source: "Context",
						label: `prefix ${choice.prior?.depth ?? 0}/${choice.prior?.contextLength ?? 0}`,
						action: `${choice.kind}${choice.reduce ? " ↓" : ""} ·1/${2 ** choice.power}`,
						prior: prior(choice.prior),
					}))
				: [],
		desk: {
			settled: Number(state.resolved),
			traders: agents.map((member) => ({
				id: member.id,
				decisions: Number(member.decisions),
				fills: Number(member.fills),
				graded: Number(member.reading?.samples ?? 0n),
				observed: member.reading?.defined ? Number(member.reading.samples) : 0,
				quality: member.reading?.mean ?? 0,
				wealth: member.wealth,
				open: Number(member.pending),
				holding: member.positions.filter(
					(position) => Number(position.holding?.qty) > 0,
				).length,
			})),
		},
		lanes: agents.map((member) => ({
			lane: member.id,
			mode: member.id === 0 ? "policy" : "virtual",
			cash: String(member.cash),
			quantity: String(
				member.positions.find(
					(position) => position.holding?.symbol === market?.symbol,
				)?.holding?.qty ?? "0",
			),
			fees: String(member.fees),
			equity: Number(member.equity),
			profit: Number(member.profit),
			at,
			action: {
				kind: String(member.last?.action?.kind ?? ""),
				power: member.last?.action?.power ?? 0,
				reduce: member.last?.action?.reduce ?? false,
			},
			issued: Number(member.decisions),
			fills: Number(member.fills),
			resolved: Number(member.reading?.samples ?? 0n),
			unresolved: Number(member.pending),
			prior: prior(member.reading),
			realized: Number(member.realized),
		})),
	};
};

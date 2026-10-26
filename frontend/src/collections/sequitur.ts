import { createStore } from "@tanstack/store";
import { trySendCommand } from "#/lib/hub-socket";
import type { Value } from "#/types/value";
import { cognitionStore } from "./cognition";

/*
The live Sequitur feed: strategy.SequiturFrame values the hub streams as
sequitur/grammar/<symbol> (data/schema.go). Each frame is the regime the
grammar closed, or the chord when none closed, whether that regime's
terminals voted enter, the absolute energy error on that tick, every
structural event its append caused, and the whole grammar after it.
*/

export type GrammarEventKind = "append" | "create" | "reuse" | "inline";

export type GrammarEvent = {
	kind: GrammarEventKind;
	token?: string;
	rule: number;
	into: number;
	from: number;
	digram: [string, string];
};

export type GrammarRule = {
	id: number;
	name: string;
	count: number;
	depth: number;
	length: number;
	symbols: string[];
	terminals: string[];
	action?: string;
};

/*
What this tick recorded for the chord. Surprise is the absolute gap
between metric energy and its running prediction, null until a prior
exists. z is that error against the symbol's earlier errors, null until
two exist. warm is grid maturity, a reading only. passed means the
regime voted enter and its brightness cleared that path's threshold.
The threshold rises after a losing exit and falls after a winning one.
The friction check itself is measurement.frictionPass, not passed.
*/
export type SequiturGate = {
	surprise: number | null;
	z: number | null;
	warm: boolean;
	/* Why the symbol is still cold: "no error yet", "history" or "falling". */
	cold?: string;
	passed: boolean;
	/* R07 ignition regime brightness cleared IgnitionMinBrightness on this tick. */
	ignition?: boolean;
};

export type FlowReading = {
	known: boolean;
	signedNetFraction?: number;
	midpointLogReturn?: number;
};

export type SequiturMeasurement = {
	canonical: boolean;
	sequiturPath: string;
	pathDrift: boolean;
	/* True only on a tick that closed a regime, so a quiet tick is not a match. */
	pathCompared?: boolean;
	frictionPass: boolean;
	/* True when the friction gate is in force for this tick. */
	frictionChecked?: boolean;
	flow?: FlowReading;
};

export type SequiturFrame = {
	symbol: string;
	epoch: string;
	tick: number;
	timestamp: string;
	active: Array<{ key: number; brightness: number }>;
	midpoint: number;
	fired: string[];
	held: number;
	gate: SequiturGate | null;
	/* The gate did not pass this chord: nothing was appended or matched. */
	blocked: boolean;
	blockedReason?: string;
	measurement?: SequiturMeasurement;
	events: GrammarEvent[];
	completed: string;
	path: string;
	/*
		How the recent tokens meet the CognitionTree: the deepest stored path
		they end on (match, depth in tokens), its action, and what paper
		trading did with it (outcome; "" when trading is off or nothing
		matched).
	*/
	policy: {
		epoch: string;
		paths: number;
		trading: boolean;
		stored: boolean;
		/* Canonical stored policy key (exact or fuzzy winner). */
		match: string;
		/* Live accumulated path on this symbol. */
		livePath?: string;
		triggerPath?: string;
		policyPath?: string;
		depth: number;
		action: string;
		outcome: string;
		similarity?: number | null;
		exactMatch?: boolean;
	};
	rules: GrammarRule[];
	start: string[];
	rawLength: number;
	grammarSymbols: number;
	violations: string[];
};

export type LoggedEvent = GrammarEvent & { tick: number; seq: number };
export type LoggedChord = {
	seq: number;
	tick: number;
	fired: string[];
	held: number;
	gate: SequiturGate | null;
	blocked: boolean;
	completed: string;
	path: string;
	stored: boolean;
	match: string;
	action: string;
	outcome: string;
	similarity?: number | null;
	exactMatch?: boolean;
	livePath?: string;
	blockedReason?: string;
};

export type SequiturFeed = {
	latest: SequiturFrame;
	frames: number;
	events: LoggedEvent[];
	chords: LoggedChord[];
	receivedAt: number;
};

export const EVENT_LOG = 400;
export const CHORD_LOG = 96;

export const sequiturStore = createStore<Record<string, SequiturFeed>>({});

/*
syncCognitionPolicyFromGrammar keeps cognition match frames aligned with the
grammar feed's policy evaluation (exact vs fuzzy, stored key vs live path).
*/
export const syncCognitionPolicyFromGrammar = (frame: SequiturFrame) => {
	const { policy } = frame;
	const phase =
		policy.policyPath ||
		policy.triggerPath ||
		policy.livePath ||
		frame.path ||
		"";
	const tokens = phase.split("/").filter(Boolean);
	cognitionStore.setState((state) => {
		const prior = state.matches[frame.symbol];

		return {
			...state,
			matches: {
				...state.matches,
				[frame.symbol]: {
					symbol: frame.symbol,
					tick: frame.tick,
					tokens,
					stored: policy.stored,
					match: policy.match,
					depth: policy.depth > 0 ? policy.depth : tokens.length,
					action: policy.outcome || policy.action || prior?.action || "",
					livePath: policy.livePath || policy.policyPath || frame.path,
					triggerPath: policy.triggerPath,
					policyPath: policy.policyPath,
					similarity: policy.similarity ?? undefined,
					exactMatch: policy.exactMatch,
					blockedReason: frame.blockedReason,
					rejected: prior?.rejected,
				},
			},
		};
	});
};

// One counter keys both logs, so React keys stay unique across frames.
let eventSeq = 0;

const isGate = (value: unknown): value is SequiturGate | null => {
	if (value === null) return true;
	if (!value || typeof value !== "object") return false;
	const gate = value as Partial<SequiturGate>;
	return (
		(gate.surprise === null || typeof gate.surprise === "number") &&
		(gate.z === null || typeof gate.z === "number") &&
		typeof gate.warm === "boolean" &&
		// A cold reading names why; a warm one carries no reason.
		(gate.warm ? gate.cold === undefined : typeof gate.cold === "string") &&
		typeof gate.passed === "boolean"
	);
};

const isFrame = (value: unknown): value is SequiturFrame => {
	if (!value || typeof value !== "object") return false;
	const frame = value as Partial<SequiturFrame>;
	return (
		typeof frame.symbol === "string" &&
		frame.symbol !== "" &&
		typeof frame.tick === "number" &&
		typeof frame.epoch === "string" &&
		Array.isArray(frame.fired) &&
		Array.isArray(frame.events) &&
		(frame.rules === undefined || Array.isArray(frame.rules)) &&
		(frame.start === undefined || Array.isArray(frame.start)) &&
		Array.isArray(frame.active) &&
		Array.isArray(frame.violations) &&
		typeof frame.blocked === "boolean" &&
		isGate(frame.gate) &&
		typeof frame.policy === "object" &&
		frame.policy !== null &&
		typeof frame.policy.trading === "boolean" &&
		typeof frame.policy.match === "string" &&
		typeof frame.policy.outcome === "string"
	);
};

const normalizeGate = (gate: unknown): SequiturGate | null => {
	if (gate === null || gate === undefined) return null;
	if (!gate || typeof gate !== "object") return null;
	const raw = gate as Partial<SequiturGate>;
	return {
		surprise: raw.surprise ?? null,
		z: raw.z ?? null,
		warm: raw.warm ?? false,
		cold: raw.warm ? undefined : (raw.cold ?? "no error yet"),
		passed: raw.passed ?? false,
		ignition: raw.ignition,
	};
};

/*
ingestTrainingStep reads a training/step Value (strategy training.go) and
feeds the sequitur store the same shape ingestSequiturFrame expects.
*/
export const ingestTrainingStep = (value: Value, now = Date.now()) => {
	if (value.origin !== "training" || value.role !== "step" || !value.scope) {
		return false;
	}

	const fired = (value.reading("fired") as string[] | undefined) ?? [];
	const active =
		(value.reading("active") as SequiturFrame["active"] | undefined) ?? [];
	const midpoint = Number(value.reading("midpoint") ?? 0);
	const reported = (value.reading("reported") as string | undefined) ?? "noop";
	const blocked = Boolean(value.reading("blocked"));
	const blockedReason =
		(value.reading("blockedReason") as string | undefined) ?? undefined;
	const livePath =
		(value.reading("livePath") as string | undefined) ??
		(value.reading("path") as string | undefined) ??
		"";

	const measurement = value.reading("measurement") as
		| SequiturMeasurement
		| undefined;

	const events =
		(value.reading("grammar/events") as GrammarEvent[] | undefined) ?? [];
	const completed =
		(value.reading("grammar/completed") as string | undefined) ?? "";
	const rules =
		(value.reading("grammar/rules") as GrammarRule[] | undefined) ?? [];
	const start =
		(value.reading("grammar/start") as string[] | undefined) ?? [];

	const wirePolicy = value.reading("policy") as
		| SequiturFrame["policy"]
		| undefined;

	const frame: SequiturFrame = {
		symbol: value.scope,
		epoch: String(value.epoch),
		tick: Number(value.tick),
		timestamp: String(value.tick),
		active,
		midpoint,
		fired,
		held: fired.length,
		gate: normalizeGate(value.reading("gate")),
		blocked,
		blockedReason,
		measurement,
		events,
		completed,
		path: livePath,
		policy: wirePolicy ?? {
			epoch: String(value.epoch),
			paths: 0,
			trading: false,
			stored: false,
			match: "",
			livePath,
			depth: 0,
			action: reported,
			outcome: "",
		},
		rules,
		start,
		rawLength: Number(value.reading("grammar/rawLength") ?? 0),
		grammarSymbols: Number(value.reading("grammar/grammarSymbols") ?? 0),
		violations: [],
	};

	const ok = ingestSequiturFrame(frame, now);

	const exploreThresholds = value.reading("exploreThresholds") as
		| Record<string, number>
		| undefined;
	if (exploreThresholds && typeof exploreThresholds === "object") {
		cognitionStore.setState((state) => ({
			...state,
			thresholds: { ...state.thresholds, ...exploreThresholds },
		}));
	}

	if (value.reading("outcome") === "exit submitted") {
		trySendCommand({ type: "s3", prefix: "model/explore" });
	}

	return ok;
};

/*
ingestSequiturFrame appends one frame to its symbol's feed. A payload that is
not a SequiturFrame is reported and dropped, never half-applied.
*/
export const ingestSequiturFrame = (payload: unknown, now = Date.now()) => {
	if (!isFrame(payload)) {
		console.error("ingestSequiturFrame: malformed sequitur frame", payload);
		return false;
	}

	const frame = payload;

	sequiturStore.setState((state) => {
		const prior = state[frame.symbol];
		/*
		A quiet tick omits the rulebook. The page keeps the book it already
		has and still takes the new gate and policy reading.
		*/
		const rules = Array.isArray(frame.rules)
			? frame.rules
			: (prior?.latest.rules ?? []);
		const start = Array.isArray(frame.start)
			? frame.start
			: (prior?.latest.start ?? []);
		const latest: SequiturFrame = { ...frame, rules, start };
		const events = (prior?.events ?? []).concat(
			frame.events.map((event) => ({
				...event,
				tick: frame.tick,
				seq: eventSeq++,
			})),
		);
		const chords = (prior?.chords ?? []).concat({
			seq: eventSeq++,
			tick: frame.tick,
			fired: frame.fired,
			held: frame.held,
			gate: frame.gate,
			blocked: frame.blocked,
			blockedReason: frame.blockedReason,
			completed: frame.completed,
			path: frame.path,
			livePath: frame.policy.livePath ?? frame.path,
			stored: frame.policy.stored,
			match: frame.policy.match,
			action: frame.policy.action,
			outcome: frame.policy.outcome,
			similarity: frame.policy.similarity,
			exactMatch: frame.policy.exactMatch,
		});

		return {
			...state,
			[frame.symbol]: {
				latest,
				frames: (prior?.frames ?? 0) + 1,
				events: events.slice(-EVENT_LOG),
				chords: chords.slice(-CHORD_LOG),
				receivedAt: now,
			},
		};
	});

	syncCognitionPolicyFromGrammar(frame);

	return true;
};

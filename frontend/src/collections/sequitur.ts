import { createStore } from "@tanstack/store";

/*
The live Sequitur feed: strategy.SequiturFrame values the hub streams as
sequitur/grammar/<symbol> (data/schema.go). Each frame is one new chord the
grid's midpoint rule proposed, what the surprise gate made of it, every
structural event its append caused (none when the gate blocked it) and the
whole grammar after it.
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
What the chord-level surprise gate read on the frame's tick
(strategy.GateReading): the resonance solver's surprise, whether the
symbol is warm (its coder's error has settled) and, while cold, why, its
causal z-score against the symbol's surprises since it warmed (null until two
exist), and whether the proposed chord passed. Null when
resonance.gate is off.
*/
export type SequiturGate = {
	surprise: number;
	z: number | null;
	warm: boolean;
	/* Why the symbol is still cold: "no error yet", "history" or "falling". */
	cold?: string;
	passed: boolean;
};

export type SequiturMeasurement = {
	canonical: boolean;
	sequiturPath: string;
	pathDrift: boolean;
	frictionPass: boolean;
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
		match: string;
		depth: number;
		action: string;
		outcome: string;
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

// One counter keys both logs, so React keys stay unique across frames.
let eventSeq = 0;

const isGate = (value: unknown): value is SequiturGate | null => {
	if (value === null) return true;
	if (!value || typeof value !== "object") return false;
	const gate = value as Partial<SequiturGate>;
	return (
		typeof gate.surprise === "number" &&
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
		Array.isArray(frame.rules) &&
		Array.isArray(frame.start) &&
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
			completed: frame.completed,
			path: frame.path,
			stored: frame.policy.stored,
			match: frame.policy.match,
			action: frame.policy.action,
			outcome: frame.policy.outcome,
		});

		return {
			...state,
			[frame.symbol]: {
				latest: frame,
				frames: (prior?.frames ?? 0) + 1,
				events: events.slice(-EVENT_LOG),
				chords: chords.slice(-CHORD_LOG),
				receivedAt: now,
			},
		};
	});

	return true;
};

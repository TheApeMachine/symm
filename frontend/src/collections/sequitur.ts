import { createStore } from "@tanstack/store";
import { triePrefix } from "#/collections/book";
import { trySendCommand } from "#/lib/hub-socket";
import type { Value } from "#/types/value";
import { cognitionStore, ingestPolicyKeys } from "./cognition";
import { stageCutReading } from "./stage-cut";

/*
Per-symbol training step readings from hub training/step frames: gate, cut,
policy match, and lit regions for the learning dashboard.
*/

export type SequiturGate = {
	surprise: number | null;
	z: number | null;
	warm: boolean;
	cold?: string;
	passed: boolean;
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
	pathCompared?: boolean;
	frictionPass: boolean;
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
	blocked: boolean;
	blockedReason?: string;
	measurement?: SequiturMeasurement;
	path: string;
	policy: {
		epoch: string;
		paths: number;
		trading: boolean;
		stored: boolean;
		match: string;
		livePath?: string;
		triggerPath?: string;
		policyPath?: string;
		depth: number;
		action: string;
		outcome: string;
		similarity?: number | null;
		exactMatch?: boolean;
	};
};

export type LoggedChord = {
	seq: number;
	tick: number;
	fired: string[];
	held: number;
	gate: SequiturGate | null;
	blocked: boolean;
	path: string;
	livePath?: string;
	stored: boolean;
	match: string;
	action: string;
	outcome: string;
	similarity?: number | null;
	exactMatch?: boolean;
	blockedReason?: string;
};

export type SequiturFeed = {
	latest: SequiturFrame;
	frames: number;
	chords: LoggedChord[];
	receivedAt: number;
};

export const CHORD_LOG = 96;

export const sequiturStore = createStore<Record<string, SequiturFeed>>({});

export const syncCognitionPolicyFromStep = (frame: SequiturFrame) => {
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

let chordSeq = 0;

const isGate = (value: unknown): value is SequiturGate | null => {
	if (value === null) return true;
	if (!value || typeof value !== "object") return false;
	const gate = value as Partial<SequiturGate>;
	return (
		(gate.surprise === null || typeof gate.surprise === "number") &&
		(gate.z === null || typeof gate.z === "number") &&
		typeof gate.warm === "boolean" &&
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
		Array.isArray(frame.active) &&
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

export const ingestTrainingStep = (value: Value, now = Date.now()) => {
	if (value.origin !== "training" || value.role !== "step" || !value.scope) {
		return false;
	}

	const cut = stageCutReading(value.scope);
	const firedFromStep = (value.reading("fired") as string[] | undefined) ?? [];
	const stepHasCut = firedFromStep.length > 0;
	const fired = stepHasCut ? firedFromStep : cut.fired;
	const active =
		(value.reading("active") as SequiturFrame["active"] | undefined) ?? [];
	const midpointFromStep = Number(value.reading("midpoint") ?? 0);
	const midpoint = stepHasCut ? midpointFromStep : cut.midpoint;
	const reported = (value.reading("reported") as string | undefined) ?? "noop";
	const blocked = Boolean(value.reading("blocked"));
	const blockedReason =
		(value.reading("blockedReason") as string | undefined) ?? undefined;
	const livePath =
		(value.reading("livePath") as string | undefined) ??
		(value.reading("path") as string | undefined) ??
		cut.path;

	const measurement = value.reading("measurement") as
		| SequiturMeasurement
		| undefined;

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
	};

	const ok = ingestSequiturFrame(frame, now);

	const exploreKeys = value.reading("exploreKeys") as string[] | undefined;
	if (
		triePrefix() === "model/paths" &&
		Array.isArray(exploreKeys) &&
		exploreKeys.length > 0
	) {
		ingestPolicyKeys(exploreKeys, String(value.epoch));
	}

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
		trySendCommand({ type: "s3", prefix: triePrefix() });
	}

	return ok;
};

export const ingestSequiturFrame = (payload: unknown, now = Date.now()) => {
	if (!isFrame(payload)) {
		console.error("ingestSequiturFrame: malformed training step frame", payload);
		return false;
	}

	const frame = payload;

	sequiturStore.setState((state) => {
		const prior = state[frame.symbol];
		const chords = (prior?.chords ?? []).concat({
			seq: chordSeq++,
			tick: frame.tick,
			fired: frame.fired,
			held: frame.held,
			gate: frame.gate,
			blocked: frame.blocked,
			blockedReason: frame.blockedReason,
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
				latest: frame,
				frames: (prior?.frames ?? 0) + 1,
				chords: chords.slice(-CHORD_LOG),
				receivedAt: now,
			},
		};
	});

	syncCognitionPolicyFromStep(frame);

	return true;
};

import { useSelector } from "@tanstack/react-store";
import { createStore } from "@tanstack/store";
import type { Book } from "#/collections/book";
import { Circular, type CircularBuffer } from "#/collections/circular";
import { cognitionStore } from "#/collections/cognition";
import { confidenceAtom, confidenceFor } from "#/collections/confidence";
import {
	closedOnBook,
	countsTowardBookPerformance,
	onBook,
	openOnBook,
	positionReadout,
} from "#/components/book-readout";
import {
	EXPLORE_PRUNE_THRESHOLD,
	isPathBookKey,
} from "#/components/learning/explore-path";
import { trySendCommand } from "#/lib/hub-socket";
import {
	deskRowsForStats,
	ingestDeskClosedLots,
	requestClosedPositionHistory,
} from "#/lib/lifecycle";

/*
Longtail samples the path-learning trajectory from live truth the hub already
publishes: desk lots, confidence, and the model/paths resistance book (plus
S3 ListPrefix for those same prefixes). Nothing here invents a parallel
training aggregate — charts stay empty until those sources have real values.
*/

export type Trend = "up" | "down" | "flat";

export type Tone = "good" | "warn" | "bad" | "info" | "neutral";

export const toneColor = (tone: Tone): string =>
	tone === "good"
		? "var(--up)"
		: tone === "warn"
			? "var(--warn)"
			: tone === "bad"
				? "var(--down)"
				: tone === "info"
					? "var(--info)"
					: "var(--f3)";

export const toneVariant = (
	tone: Tone,
): "success" | "warning" | "error" | "info" | "disabled" =>
	tone === "good"
		? "success"
		: tone === "warn"
			? "warning"
			: tone === "bad"
				? "error"
				: tone === "info"
					? "info"
					: "disabled";

export const alertVariant = (
	tone: Tone,
): "success" | "warning" | "error" | "info" =>
	tone === "good"
		? "success"
		: tone === "warn"
			? "warning"
			: tone === "bad"
				? "error"
				: "info";

export const winRateTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value >= 0.55
			? "good"
			: value >= 0.45
				? "warn"
				: "bad";

export const returnTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value > 0
			? "good"
			: value >= -0.01
				? "warn"
				: "bad";

export const matchTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value >= 0.5
			? "good"
			: value >= 0.2
				? "warn"
				: "bad";

export const resistanceTone = (value: number | null, floor: number): Tone =>
	value === null
		? "neutral"
		: value <= floor
			? "good"
			: value <= 0.5
				? "info"
				: value <= EXPLORE_PRUNE_THRESHOLD
					? "warn"
					: "bad";

export type BookDeskStats = {
	closed: number;
	open: number;
	wins: number;
	winRate: number | null;
	meanReturn: number | null;
	medianReturn: number | null;
	worstReturn: number | null;
	avgHoldNs: number | null;
};

/*
PathBookSnapshot is the resistance trie as listed from model/paths. Developed
leaves are still in the book (≤ prune ceiling). Promoted leaves cleared the
paper floor. Added/removed are churn vs the previous sample's key set.
*/
export type PathBookSnapshot = {
	developed: number | null;
	promoted: number | null;
	meanResistance: number | null;
	added: number | null;
	removed: number | null;
};

export type PolicySnapshot = {
	checks: number;
	hits: number;
	hitRate: number | null;
};

/*
ExitMix buckets closed-lot triggers into profit / stop / policy / other. Counts
are cumulative for the book at sample time — never an unbounded map.
*/
export type ExitMix = {
	profit: number;
	stop: number;
	policy: number;
	other: number;
	total: number;
};

export type OpenLotTelemetry = {
	open: number | null;
	invalidation: number | null;
	trailing: number | null;
	damage: number | null;
	armed: number | null;
	maxProximity: number | null;
};

export type LongtailSample = {
	at: number;
	paper: BookDeskStats;
	virtual: BookDeskStats;
	real: BookDeskStats;
	/** Authoritative paper/real strip from the confidence frame. */
	paperWin: number | null;
	paperEdge: number | null;
	realWin: number | null;
	realEdge: number | null;
	realArmed: number | null;
	eligibleResistance: number;
	paths: PathBookSnapshot;
	policy: PolicySnapshot;
	exits: {
		virtual: ExitMix;
		paper: ExitMix;
		real: ExitMix;
	};
	stops: {
		virtual: OpenLotTelemetry;
		paper: OpenLotTelemetry;
		real: OpenLotTelemetry;
	};
};

export type Insight = {
	id: string;
	title: string;
	body: string;
	tone: Tone;
};

export type Verdict = {
	label: string;
	tone: Tone;
};

export const pct = (value: number) => `${(100 * value).toFixed(1)}%`;

export const emptyBookStats = (): BookDeskStats => ({
	closed: 0,
	open: 0,
	wins: 0,
	winRate: null,
	meanReturn: null,
	medianReturn: null,
	worstReturn: null,
	avgHoldNs: null,
});

export const emptyExitMix = (): ExitMix => ({
	profit: 0,
	stop: 0,
	policy: 0,
	other: 0,
	total: 0,
});

export const emptyStops = (): OpenLotTelemetry => ({
	open: null,
	invalidation: null,
	trailing: null,
	damage: null,
	armed: null,
	maxProximity: null,
});

export const emptyPaths = (): PathBookSnapshot => ({
	developed: null,
	promoted: null,
	meanResistance: null,
	added: null,
	removed: null,
});

export const emptyPolicy = (): PolicySnapshot => ({
	checks: 0,
	hits: 0,
	hitRate: null,
});

const median = (values: number[]): number => {
	const sorted = [...values].sort((a, b) => a - b);
	const mid = Math.floor(sorted.length / 2);
	return sorted.length % 2 === 0
		? (sorted[mid - 1] + sorted[mid]) / 2
		: sorted[mid];
};

const mean = (values: number[]): number | null =>
	values.length > 0
		? values.reduce((sum, value) => sum + value, 0) / values.length
		: null;

/*
classifyExit folds a trigger string into the four mix buckets used by charts.
*/
export const classifyExit = (trigger: string): keyof Omit<ExitMix, "total"> => {
	const label = trigger.toLowerCase();
	if (
		label.includes("exit branch") ||
		label.includes("learned_exit") ||
		label.includes("policy exit")
	) {
		return "policy";
	}
	if (
		label.includes("cleared friction") ||
		label.includes("pct profit") ||
		label.includes("trailing") ||
		label.includes("harvest") ||
		(label.includes("profit") && !label.includes("stop"))
	) {
		return "profit";
	}
	if (
		label.includes("invalidation") ||
		label.includes("friction dip") ||
		label.includes("stop") ||
		label.includes("exhaustion") ||
		label.includes("evaporation") ||
		label.includes("drag")
	) {
		return "stop";
	}
	return "other";
};

export const summarizeBook = (
	rows: Record<string, unknown>[],
	book: Book,
): BookDeskStats => {
	const onThisBook = rows.filter((row) => onBook(row, book));
	const settled = closedOnBook(rows, book).filter(
		(row) => positionReadout(row).returnPct !== null,
	);
	const scored = settled.filter((row) =>
		countsTowardBookPerformance(row, book),
	);
	const returns = scored.map((row) => positionReadout(row).returnPct as number);
	const wins = returns.filter((value) => value > 0).length;
	const holds = settled
		.map((row) => positionReadout(row).holdNs)
		.filter((value): value is number => value !== null && value > 0);

	return {
		closed: scored.length,
		open: openOnBook(onThisBook, book).length,
		wins,
		winRate: returns.length > 0 ? wins / returns.length : null,
		meanReturn: mean(returns),
		medianReturn: returns.length > 0 ? median(returns) : null,
		worstReturn: returns.length > 0 ? Math.min(...returns) : null,
		avgHoldNs: mean(holds),
	};
};

/*
summarizeStops folds open-lot DynamicStoploss for one book. Legacy field names
(invalidation / trailing / damage) map to pressure, floor proximity, and
magnitude ratio so longtail history stays compatible. Exit reason maps are not
stored here — use summarizeExits so samples stay fixed-size.
*/
export const summarizeStops = (
	rows: Record<string, unknown>[],
	book: Book,
): OpenLotTelemetry => {
	const open = rows.filter(
		(row) => onBook(row, book) && !positionReadout(row).closed,
	);
	const pressureSamples: number[] = [];
	const proximitySamples: number[] = [];
	const magnitudeSamples: number[] = [];
	let armed = 0;
	let maxProximity = 0;
	let withState = 0;

	for (const row of open) {
		const stoploss =
			row.stoploss && typeof row.stoploss === "object"
				? (row.stoploss as Record<string, unknown>)
				: null;
		if (!stoploss) continue;
		withState += 1;

		const state = stoploss.state;
		const proximity =
			typeof stoploss.proximity === "number" &&
			Number.isFinite(stoploss.proximity)
				? stoploss.proximity
				: 0;
		const downside =
			typeof stoploss.downside_pressure === "number" &&
			Number.isFinite(stoploss.downside_pressure)
				? stoploss.downside_pressure
				: 0;
		const magnitude =
			typeof stoploss.downside_magnitude_ratio === "number" &&
			Number.isFinite(stoploss.downside_magnitude_ratio)
				? stoploss.downside_magnitude_ratio
				: 0;

		if (downside > 0) pressureSamples.push(downside);
		if (stoploss.has_floor === true && proximity > 0) {
			proximitySamples.push(proximity);
		}
		if (magnitude > 0) magnitudeSamples.push(magnitude);

		const rowArmed =
			state === "Stalled" ||
			state === "Dangerous" ||
			state === "Triggered" ||
			proximity >= 0.7 ||
			downside >= 0.7;
		if (rowArmed) armed += 1;
		if (proximity > maxProximity) maxProximity = proximity;
	}

	return {
		open: open.length > 0 ? open.length : null,
		invalidation: mean(pressureSamples),
		trailing: mean(proximitySamples),
		damage: mean(magnitudeSamples),
		armed: withState > 0 ? armed : null,
		maxProximity: withState > 0 && maxProximity > 0 ? maxProximity : null,
	};
};

export const summarizeExits = (
	rows: Record<string, unknown>[],
	book: Book,
): ExitMix => {
	const mix = emptyExitMix();
	const closed = closedOnBook(rows, book);

	for (const row of closed) {
		const triggers: string[] = [];
		if (Array.isArray(row.outcome_triggers)) {
			for (const trigger of row.outcome_triggers) {
				if (typeof trigger === "string" && trigger !== "") {
					triggers.push(trigger);
				}
			}
		}
		if (
			triggers.length === 0 &&
			typeof row.exit_trigger === "string" &&
			row.exit_trigger !== ""
		) {
			triggers.push(row.exit_trigger);
		}
		if (triggers.length === 0) continue;

		for (const trigger of triggers) {
			const bucket = classifyExit(trigger);
			mix[bucket] += 1;
			mix.total += 1;
		}
	}

	return mix;
};

/*
summarizePathBook reads the cognition resistance map. Keys without a reported
resistance are counted as developed but skip the mean — never invent 0.5.
*/
export const summarizePathBook = (
	thresholds: Record<string, number>,
	policyKeys: string[],
	eligible: number,
	priorKeys: Set<string> | null,
): PathBookSnapshot => {
	const keys = policyKeys.filter(isPathBookKey);
	if (keys.length === 0 && Object.keys(thresholds).length === 0) {
		return emptyPaths();
	}

	const active =
		keys.length > 0 ? keys : Object.keys(thresholds).filter(isPathBookKey);
	const resistances: number[] = [];
	let promoted = 0;
	let developed = 0;

	for (const key of active) {
		const resistance = thresholds[key];
		if (typeof resistance !== "number" || !Number.isFinite(resistance)) {
			developed += 1;
			continue;
		}
		if (resistance > EXPLORE_PRUNE_THRESHOLD) continue;
		developed += 1;
		resistances.push(resistance);
		if (resistance <= eligible) promoted += 1;
	}

	const current = new Set(active);
	let added: number | null = null;
	let removed: number | null = null;
	if (priorKeys !== null) {
		added = 0;
		removed = 0;
		for (const key of current) {
			if (!priorKeys.has(key)) added += 1;
		}
		for (const key of priorKeys) {
			if (!current.has(key)) removed += 1;
		}
	}

	return {
		developed: developed > 0 ? developed : null,
		promoted: developed > 0 ? promoted : null,
		meanResistance: mean(resistances),
		added,
		removed,
	};
};

export const summarizePolicy = (
	matches: Record<string, { stored?: boolean }>,
): PolicySnapshot => {
	const rows = Object.values(matches);
	if (rows.length === 0) return emptyPolicy();

	let hits = 0;
	for (const match of rows) {
		if (match.stored === true) hits += 1;
	}

	return {
		checks: rows.length,
		hits,
		hitRate: hits / rows.length,
	};
};

/*
tierStats picks the selected book's desk stats. Paper/real win and edge prefer
the confidence frame when it has closed lots — that is what arms real.
*/
export const tierStats = (
	sample: LongtailSample,
	book: Book,
): {
	stats: BookDeskStats;
	winRate: number | null;
	edge: number | null;
	armed: number | null;
} => {
	const stats = sample[book];
	if (book === "paper") {
		return {
			stats,
			winRate: sample.paperWin ?? stats.winRate,
			edge: sample.paperEdge ?? stats.meanReturn,
			armed: null,
		};
	}
	if (book === "real") {
		return {
			stats,
			winRate: sample.realWin ?? stats.winRate,
			edge: sample.realEdge ?? stats.meanReturn,
			armed: sample.realArmed,
		};
	}
	return {
		stats,
		winRate: stats.winRate,
		edge: stats.meanReturn,
		armed: null,
	};
};

const trendOf = (current: number | null, prior: number | null): Trend => {
	if (
		current === null ||
		prior === null ||
		!Number.isFinite(current) ||
		!Number.isFinite(prior)
	) {
		return "flat";
	}
	if (Math.abs(current - prior) < 0.01) return "flat";
	return current > prior ? "up" : "down";
};

/*
signFlips counts how often the selected book's edge crossed zero across the
recent window — high flip rate with near-zero mean edge is thrash.
*/
const signFlips = (samples: LongtailSample[], book: Book): number => {
	let flips = 0;
	let prior: number | null = null;
	for (const sample of samples) {
		const edge = tierStats(sample, book).edge;
		if (edge === null) continue;
		if (prior !== null && Math.sign(prior) !== 0 && Math.sign(edge) !== 0) {
			if (Math.sign(prior) !== Math.sign(edge)) flips += 1;
		}
		prior = edge;
	}
	return flips;
};

/*
verdict answers improving / worsening / thrashing for the selected tier.
Path-book churn alone never paints green when the book's edge is still bad.
*/
export const verdict = (samples: LongtailSample[], book: Book): Verdict => {
	if (samples.length < 2) {
		return { label: "Warming up", tone: "neutral" };
	}

	const latest = samples[samples.length - 1];
	const prior = samples[Math.max(0, samples.length - 16)];
	const now = tierStats(latest, book);
	const then = tierStats(prior, book);

	if (now.edge === null && now.winRate === null) {
		const paths = latest.paths;
		if (paths.developed !== null) {
			const churn =
				(paths.added ?? 0) + (paths.removed ?? 0) >
				Math.max(2, (paths.developed ?? 0) * 0.15);
			if (churn && (paths.promoted ?? 0) === (prior.paths.promoted ?? 0)) {
				return { label: "Thrashing paths", tone: "warn" };
			}
			return { label: "Path book live", tone: "info" };
		}
		return { label: "Warming up", tone: "neutral" };
	}

	const edgeTone = returnTone(now.edge);
	const winTone = winRateTone(now.winRate);
	const trend = trendOf(now.edge, then.edge);
	const recent = samples.slice(-24);
	const flips = signFlips(recent, book);
	const pathChurn =
		(latest.paths.added ?? 0) + (latest.paths.removed ?? 0) >=
		Math.max(3, (latest.paths.developed ?? 0) * 0.2);

	const losing = edgeTone === "bad" || winTone === "bad";
	const winning = edgeTone === "good" && winTone !== "bad";

	/*
	Repeated zero-crossings without a clear improving win are thrash — the
	book is oscillating, not learning a direction.
	*/
	if (flips >= 3 && !(winning && trend === "up")) {
		return { label: "Thrashing", tone: "bad" };
	}
	if (pathChurn && trend === "flat" && edgeTone !== "good") {
		return { label: "Thrashing", tone: "warn" };
	}

	if (losing) {
		if (trend === "down") return { label: "Degrading", tone: "bad" };
		if (trend === "up") return { label: "Recovering", tone: "warn" };
		return { label: "Stuck", tone: "bad" };
	}

	if (winning) {
		if (trend === "down") return { label: "Slipping", tone: "warn" };
		if (trend === "up") return { label: "Improving", tone: "good" };
		return { label: "Stable", tone: "info" };
	}

	return { label: "Mixed", tone: "warn" };
};

export const buildInsights = (
	samples: LongtailSample[],
	book: Book,
): Insight[] => {
	const latest = samples[samples.length - 1];

	if (!latest) {
		return [
			{
				id: "warmup",
				title: "Collecting baseline",
				body: `Waiting on ${book} closed lots, confidence, and the model/paths book from the hub.`,
				tone: "neutral",
			},
		];
	}

	const prior = samples[Math.max(0, samples.length - 16)];
	const tier = tierStats(latest, book);
	const insights: Insight[] = [];

	insights.push({
		id: "tier",
		title: `${book} trajectory`,
		body:
			tier.edge === null && tier.winRate === null
				? `No settled ${book} lots yet that count toward edge.`
				: `Win ${tier.winRate === null ? "—" : pct(tier.winRate)}, edge ${tier.edge === null ? "—" : pct(tier.edge)}, ${tier.stats.closed} settled · ${tier.stats.open} open${tier.armed !== null ? ` · real ${tier.armed >= 0.5 ? "armed" : "halted"}` : ""}.`,
		tone:
			tier.edge === null
				? "neutral"
				: returnTone(tier.edge) === "good" &&
						winRateTone(tier.winRate) !== "bad"
					? "good"
					: returnTone(tier.edge),
	});

	const paths = latest.paths;
	if (paths.developed === null) {
		insights.push({
			id: "paths",
			title: "Path book",
			body: "model/paths has not listed yet. Request S3 prefix model/paths or wait for a live paths envelope.",
			tone: "neutral",
		});
	} else {
		const churn = (paths.added ?? 0) + (paths.removed ?? 0);
		const mean =
			paths.meanResistance === null ? "—" : paths.meanResistance.toFixed(2);
		const meanTrend = trendOf(paths.meanResistance, prior.paths.meanResistance);
		insights.push({
			id: "paths",
			title: "Path book",
			body: `${paths.developed} developed, ${paths.promoted ?? 0} at ≤${latest.eligibleResistance.toFixed(2)} paper floor, mean resistance ${mean} (${meanTrend}). Churn +${paths.added ?? 0}/−${paths.removed ?? 0}${churn >= 3 ? " — watch thrash" : ""}.`,
			tone:
				(paths.promoted ?? 0) > 0 && meanTrend === "down"
					? "good"
					: churn >= 3
						? "warn"
						: "info",
		});
	}

	const policy = latest.policy;
	insights.push({
		id: "policy",
		title: "Policy hit",
		body:
			policy.checks === 0
				? "No live symbol matches yet."
				: `${pct(policy.hitRate ?? 0)} of ${policy.checks} live symbol cuts landed on a stored path (${policy.hits} hits).`,
		tone: matchTone(policy.hitRate),
	});

	const exits = latest.exits[book];
	if (exits.total === 0) {
		insights.push({
			id: "exits",
			title: "Exit mix",
			body: `No closed ${book} exit triggers yet.`,
			tone: "neutral",
		});
	} else {
		insights.push({
			id: "exits",
			title: "Exit mix",
			body: `${exits.profit} profit, ${exits.stop} stop, ${exits.policy} policy, ${exits.other} other (${exits.total} triggers).`,
			tone:
				exits.profit > exits.stop
					? "good"
					: exits.stop > exits.profit
						? "warn"
						: "info",
		});
	}

	const stops = latest.stops[book];
	insights.push({
		id: "stops",
		title: "Stop posture",
		body:
			stops.open === null || stops.open === 0
				? `No open ${book} lots priced.`
				: `${stops.open} open; invalidation ${stops.invalidation === null ? "—" : pct(stops.invalidation)}, trail ${stops.trailing === null ? "—" : pct(stops.trailing)}, damage ${stops.damage === null ? "—" : pct(stops.damage)}; nearest ${stops.maxProximity === null ? "—" : pct(stops.maxProximity)}.`,
		tone:
			stops.maxProximity === null
				? "neutral"
				: stops.maxProximity >= 1
					? "bad"
					: stops.maxProximity >= 0.7
						? "warn"
						: "good",
	});

	return insights;
};

export const longtailStore = createStore({
	samples: [] as LongtailSample[],
	lastSampleAt: 0,
	sourcesReady: false,
});

export const CHART_SAMPLES = 240;
const SAMPLE_EVERY_MS = 5_000;
const EXIT_TRIGGER_CAP = 8;

let chartWindow = Circular<LongtailSample>(CHART_SAMPLES);
/** Prior path-key set for churn — replaced each sample, never appended. */
let priorPathKeys: Set<string> | null = null;
let sourcesRequested = false;

const publishWindow = (lastSampleAt?: number) => {
	const samples = chartWindow.values();
	longtailStore.setState((state) => ({
		samples,
		lastSampleAt: lastSampleAt ?? samples[samples.length - 1]?.at ?? 0,
		sourcesReady: state.sourcesReady,
	}));
};

const installWindow = (rows: LongtailSample[]) => {
	chartWindow = Circular<LongtailSample>(CHART_SAMPLES);
	for (const row of rows) chartWindow.push(row);
	publishWindow();
};

const pushSample = (sample: LongtailSample) => {
	chartWindow.push(sample);
	publishWindow(sample.at);
};

/*
requestLongtailSources asks the hub for closed lots on every tier (via the
shared history helper) plus the path resistance book. Bounded ListPrefix
replies land through the existing paths/closed envelopes.
*/
export const requestLongtailSources = () => {
	if (sourcesRequested) return;
	sourcesRequested = true;

	requestClosedPositionHistory();
	trySendCommand({ type: "s3", prefix: "model/paths" });

	longtailStore.setState((state) => ({ ...state, sourcesReady: true }));
};

export const resetLongtailSourcesRequest = () => {
	sourcesRequested = false;
	longtailStore.setState((state) => ({ ...state, sourcesReady: false }));
};

const captureSample = (at: number): LongtailSample => {
	const desk = deskRowsForStats();
	ingestDeskClosedLots(desk);

	const frame = confidenceAtom.get();
	const cognition = cognitionStore.state;
	const eligible =
		typeof frame.eligible_resistance === "number" &&
		Number.isFinite(frame.eligible_resistance)
			? frame.eligible_resistance
			: 0.2;

	const paths = summarizePathBook(
		cognition.thresholds,
		cognition.policyKeys,
		eligible,
		priorPathKeys,
	);

	const activeKeys =
		cognition.policyKeys.length > 0
			? cognition.policyKeys.filter(isPathBookKey)
			: Object.keys(cognition.thresholds).filter(isPathBookKey);
	priorPathKeys = new Set(activeKeys);

	const paperConf = confidenceFor(frame, "paper");
	const realConf = confidenceFor(frame, "real");

	return {
		at,
		paper: summarizeBook(desk, "paper"),
		virtual: summarizeBook(desk, "virtual"),
		real: summarizeBook(desk, "real"),
		paperWin: paperConf.closed > 0 ? paperConf.win_rate : null,
		paperEdge: paperConf.closed > 0 ? paperConf.edge : null,
		realWin: realConf.closed > 0 ? realConf.win_rate : null,
		realEdge: realConf.closed > 0 ? realConf.edge : null,
		realArmed: frame.real_ceiling ? (realConf.armed ? 1 : 0) : null,
		eligibleResistance: eligible,
		paths,
		policy: summarizePolicy(cognition.matches),
		exits: {
			virtual: summarizeExits(desk, "virtual"),
			paper: summarizeExits(desk, "paper"),
			real: summarizeExits(desk, "real"),
		},
		stops: {
			virtual: summarizeStops(desk, "virtual"),
			paper: summarizeStops(desk, "paper"),
			real: summarizeStops(desk, "real"),
		},
	};
};

const recordLongtailSample = () => {
	pushSample(captureSample(Date.now()));
};

const maybeRecordLongtailSample = () => {
	const { lastSampleAt, samples } = longtailStore.state;
	const now = Date.now();
	if (samples.length > 0 && now - lastSampleAt < SAMPLE_EVERY_MS) {
		return;
	}
	recordLongtailSample();
};

let recorderHandle: number | null = null;
let historyBuffer: CircularBuffer<LongtailSample> | null = null;

const finiteOrNull = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const countOrZero = (value: unknown): number => {
	const count = finiteOrNull(value);
	return count === null ? 0 : count;
};

const bookFromHistory = (value: unknown): BookDeskStats | null => {
	if (!value || typeof value !== "object") return null;
	const row = value as Record<string, unknown>;
	return {
		closed: countOrZero(row.closed),
		open: countOrZero(row.open),
		wins: countOrZero(row.wins),
		winRate: finiteOrNull(row.winRate),
		meanReturn: finiteOrNull(row.meanReturn),
		medianReturn: finiteOrNull(row.medianReturn),
		worstReturn: finiteOrNull(row.worstReturn),
		avgHoldNs: finiteOrNull(row.avgHoldNs),
	};
};

const pathsFromHistory = (value: unknown): PathBookSnapshot => {
	if (!value || typeof value !== "object") return emptyPaths();
	const row = value as Record<string, unknown>;
	return {
		developed: finiteOrNull(row.developed),
		promoted: finiteOrNull(row.promoted),
		meanResistance: finiteOrNull(row.meanResistance ?? row.meanThreshold),
		added: finiteOrNull(row.added),
		removed: finiteOrNull(row.removed),
	};
};

const policyFromHistory = (value: unknown): PolicySnapshot => {
	if (!value || typeof value !== "object") return emptyPolicy();
	const row = value as Record<string, unknown>;
	const checks = countOrZero(row.checks);
	const hits = countOrZero(row.hits);
	return {
		checks,
		hits,
		hitRate: finiteOrNull(row.hitRate) ?? (checks > 0 ? hits / checks : null),
	};
};

const exitMixFromHistory = (value: unknown): ExitMix => {
	if (!value || typeof value !== "object") return emptyExitMix();
	const row = value as Record<string, unknown>;
	return {
		profit: countOrZero(row.profit),
		stop: countOrZero(row.stop),
		policy: countOrZero(row.policy),
		other: countOrZero(row.other),
		total: countOrZero(row.total),
	};
};

const stopsFromHistory = (value: unknown): OpenLotTelemetry => {
	if (!value || typeof value !== "object") return emptyStops();
	const row = value as Record<string, unknown>;
	return {
		open: finiteOrNull(row.open),
		invalidation: finiteOrNull(row.invalidation),
		trailing: finiteOrNull(row.trailing),
		damage: finiteOrNull(row.damage),
		armed: finiteOrNull(row.armed),
		maxProximity: finiteOrNull(row.maxProximity),
	};
};

const historySample = (value: unknown): LongtailSample | null => {
	if (!value || typeof value !== "object") return null;
	const row = value as Record<string, unknown>;
	const at = finiteOrNull(row.at);
	const paper = bookFromHistory(row.paper);
	const virtual = bookFromHistory(row.virtual);
	const real = bookFromHistory(row.real) ?? emptyBookStats();
	if (at === null || paper === null || virtual === null) return null;

	const exits =
		row.exits && typeof row.exits === "object"
			? (row.exits as Record<string, unknown>)
			: {};
	const stops =
		row.stops && typeof row.stops === "object"
			? (row.stops as Record<string, unknown>)
			: {};

	return {
		at,
		paper,
		virtual,
		real,
		paperWin: finiteOrNull(row.paperWin),
		paperEdge: finiteOrNull(row.paperEdge),
		realWin: finiteOrNull(row.realWin),
		realEdge: finiteOrNull(row.realEdge),
		realArmed: finiteOrNull(row.realArmed),
		eligibleResistance: finiteOrNull(row.eligibleResistance) ?? 0.2,
		paths: pathsFromHistory(row.paths ?? row.training),
		policy: policyFromHistory(row.policy),
		exits: {
			virtual: exitMixFromHistory(exits.virtual),
			paper: exitMixFromHistory(exits.paper),
			real: exitMixFromHistory(exits.real),
		},
		stops: {
			virtual: stopsFromHistory(stops.virtual),
			paper: stopsFromHistory(stops.paper),
			real: stopsFromHistory(stops.real),
		},
	};
};

export const beginLongtailHistory = () => {
	historyBuffer = Circular<LongtailSample>(CHART_SAMPLES);
};

export const ingestLongtailHistory = (payload: unknown): boolean => {
	if (historyBuffer === null || !Array.isArray(payload)) return false;

	let accepted = 0;
	for (const item of payload) {
		const sample = historySample(item);
		if (!sample) continue;
		historyBuffer.push(sample);
		accepted += 1;
	}
	return accepted > 0;
};

export const endLongtailHistory = () => {
	if (historyBuffer === null) return;

	const seeded = historyBuffer.values().sort((a, b) => a.at - b.at);
	historyBuffer = null;
	if (seeded.length === 0) return;

	installWindow(seeded);
	maybeRecordLongtailSample();
};

const windowIsWarmupOnly = (samples: LongtailSample[]): boolean =>
	samples.length === 0 ||
	(samples.length <= 2 &&
		samples.every(
			(sample) =>
				sample.paper.closed === 0 &&
				sample.virtual.closed === 0 &&
				sample.real.closed === 0 &&
				sample.paths.developed === null,
		));

/*
seedFromClosedLots builds a bounded cumulative trajectory from desk/S3 closed
lots so the charts show improving/worsening before the live 5s window fills.
Replaces only an empty or warmup-only window — never a live series.
*/
export const seedFromClosedLots = () => {
	if (!windowIsWarmupOnly(chartWindow.values())) return false;

	const rows = deskRowsForStats().filter((row) => positionReadout(row).closed);
	if (rows.length < 2) return false;

	const dated = rows
		.map((row) => {
			const closedAt = positionReadout(row).closedAt;
			const at = closedAt ? Date.parse(closedAt) : Number.NaN;
			return { row, at: Number.isFinite(at) ? at : 0 };
		})
		.sort((a, b) => a.at - b.at);

	const step = Math.max(1, Math.ceil(dated.length / CHART_SAMPLES));
	const samples: LongtailSample[] = [];
	const cumulative: Record<string, unknown>[] = [];

	for (let i = 0; i < dated.length; i++) {
		cumulative.push(dated[i].row);
		if ((i + 1) % step !== 0 && i !== dated.length - 1) continue;

		const at =
			dated[i].at || Date.now() - (dated.length - 1 - i) * SAMPLE_EVERY_MS;
		samples.push({
			at,
			paper: summarizeBook(cumulative, "paper"),
			virtual: summarizeBook(cumulative, "virtual"),
			real: summarizeBook(cumulative, "real"),
			paperWin: null,
			paperEdge: null,
			realWin: null,
			realEdge: null,
			realArmed: null,
			eligibleResistance: confidenceAtom.get().eligible_resistance ?? 0.2,
			paths: emptyPaths(),
			policy: emptyPolicy(),
			exits: {
				virtual: summarizeExits(cumulative, "virtual"),
				paper: summarizeExits(cumulative, "paper"),
				real: summarizeExits(cumulative, "real"),
			},
			stops: {
				virtual: emptyStops(),
				paper: emptyStops(),
				real: emptyStops(),
			},
		});
	}

	if (samples.length < 2) return false;

	installWindow(samples);
	return true;
};

/*
topExitTriggers returns up to EXIT_TRIGGER_CAP reasons for the selected book,
sorted by count. Used by the rail — not stored inside samples.
*/
export const topExitTriggers = (
	rows: Record<string, unknown>[],
	book: Book,
): Array<{ trigger: string; count: number }> => {
	const counts = new Map<string, number>();

	for (const row of closedOnBook(rows, book)) {
		const triggers: string[] = [];
		if (Array.isArray(row.outcome_triggers)) {
			for (const trigger of row.outcome_triggers) {
				if (typeof trigger === "string" && trigger !== "") {
					triggers.push(trigger);
				}
			}
		}
		if (
			triggers.length === 0 &&
			typeof row.exit_trigger === "string" &&
			row.exit_trigger !== ""
		) {
			triggers.push(row.exit_trigger);
		}
		for (const trigger of triggers) {
			counts.set(trigger, (counts.get(trigger) ?? 0) + 1);
		}
	}

	return [...counts.entries()]
		.map(([trigger, count]) => ({ trigger, count }))
		.sort((a, b) => b.count - a.count)
		.slice(0, EXIT_TRIGGER_CAP);
};

export const ensureLongtailRecorder = () => {
	if (recorderHandle !== null) {
		return;
	}

	requestLongtailSources();
	seedFromClosedLots();
	recordLongtailSample();
	recorderHandle = window.setInterval(() => {
		seedFromClosedLots();
		maybeRecordLongtailSample();
	}, SAMPLE_EVERY_MS);
};

export const resetLongtailRecorder = () => {
	if (recorderHandle !== null) {
		window.clearInterval(recorderHandle);
		recorderHandle = null;
	}
	historyBuffer = null;
	priorPathKeys = null;
	resetLongtailSourcesRequest();
	chartWindow = Circular<LongtailSample>(CHART_SAMPLES);
	longtailStore.setState(() => ({
		samples: [],
		lastSampleAt: 0,
		sourcesReady: false,
	}));
};

export const useLongtailReport = () =>
	useSelector(longtailStore, (state) => state);

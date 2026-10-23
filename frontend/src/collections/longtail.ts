import { createStore } from "@tanstack/store";
import { useSelector } from "@tanstack/react-store";
import { signals } from "#/collections/app";
import type { Book } from "#/collections/book";
import { type SequiturFeed, sequiturStore } from "#/collections/sequitur";
import {
	closedOnBook,
	onBook,
	openOnBook,
	positionReadout,
} from "#/components/book-readout";
import { positionRows } from "#/components/learning/open-positions";
import { ingestDeskClosedLots } from "#/lib/lifecycle";

export type Trend = "up" | "down" | "flat";

/*
Tone is the contextual colour a metric paints with: good is green, warn is
amber, bad is red, info is blue, neutral is grey. A metric that is not doing
well must never paint green — each tone function below encodes the metric's
own thresholds, so the colour always says what the number means.
*/
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

/* The UI library's semantic variants, so meters, badges and alerts paint the same way. */
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

/*
Alerts have no disabled variant: a neutral reading is still a reading, so it
paints as info rather than disappearing into a dead band.
*/
export const alertVariant = (
	tone: Tone,
): "success" | "warning" | "error" | "info" =>
	tone === "good" ? "success" : tone === "warn" ? "warning" : tone === "bad" ? "error" : "info";

/*
Per-metric tone thresholds. A null reading is neutral: absence of data is
neither good nor bad, and painting it green or red would invent a verdict.
*/
export const winRateTone = (value: number | null): Tone =>
	value === null ? "neutral" : value >= 0.55 ? "good" : value >= 0.45 ? "warn" : "bad";

export const returnTone = (value: number | null): Tone =>
	value === null ? "neutral" : value > 0 ? "good" : value >= -0.01 ? "warn" : "bad";

export const gatePassTone = (value: number | null): Tone =>
	value === null ? "neutral" : value >= 0.7 ? "good" : value >= 0.4 ? "warn" : "bad";

/* Surprise is the predictive coder's error: lower is better, so the scale inverts. */
export const surpriseTone = (value: number | null): Tone =>
	value === null ? "neutral" : value <= 0.5 ? "good" : value <= 2 ? "warn" : "bad";

/* A maturing grammar reuses its rules; one that mostly creates new rules has not found structure. */
export const reuseTone = (value: number | null): Tone =>
	value === null ? "neutral" : value >= 0.5 ? "good" : value >= 0.25 ? "warn" : "bad";

/* The share of grammar rules that carry a stored enter/exit action: how much of the grammar is trained. */
export const coverageTone = (value: number | null): Tone =>
	value === null ? "neutral" : value >= 0.5 ? "good" : value >= 0.25 ? "warn" : "bad";

/* How often the live token path ends on a stored policy path. */
export const matchTone = (value: number | null): Tone =>
	value === null ? "neutral" : value >= 0.5 ? "good" : value >= 0.2 ? "warn" : "bad";

/* Coders whose error has settled: the gate can only trust warm symbols. */
export const warmTone = (value: number | null): Tone =>
	value === null ? "neutral" : value >= 0.7 ? "good" : value >= 0.4 ? "warn" : "bad";

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
One snapshot of the training system, aggregated across every symbol's live
grammar feed: the grammar's structure, the mix of structural events its last
append caused, the surprise gate's readings, and how the stored policy met the
live token path.
*/
export type TrainingSample = {
	feeds: number;
	rules: number | null;
	grammarSymbols: number | null;
	rawLength: number | null;
	avgRuleDepth: number | null;
	maxRuleDepth: number | null;
	actionCoverage: number | null;
	createRate: number | null;
	reuseRate: number | null;
	inlineRate: number | null;
	gatePassRate: number | null;
	blockedRate: number | null;
	warmRate: number | null;
	avgSurprise: number | null;
	avgZ: number | null;
	policyStoredRate: number | null;
	policyMatchRate: number | null;
	avgMatchDepth: number | null;
	outcomeRate: number | null;
	violations: number | null;
};

export type LongtailSample = {
	at: number;
	paper: BookDeskStats;
	virtual: BookDeskStats;
	training: TrainingSample;
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

/* Display formatting: fractions as percentages, signed where the sign matters. */
export const pct = (value: number) => `${(100 * value).toFixed(1)}%`;
export const signedPct = (value: number) =>
	`${value >= 0 ? "+" : ""}${(100 * value).toFixed(1)}%`;
export const fixed = (value: number, digits = 2) => value.toFixed(digits);

const median = (values: number[]): number => {
	const sorted = [...values].sort((a, b) => a - b);
	const mid = Math.floor(sorted.length / 2);
	return sorted.length % 2 === 0
		? (sorted[mid - 1] + sorted[mid]) / 2
		: sorted[mid];
};

const mean = (values: number[]): number | null =>
	values.length > 0 ? values.reduce((sum, value) => sum + value, 0) / values.length : null;

/*
summarizeBook reads one book's desk rows into its round-trip statistics.
Returns are only counted for closed lots the desk has priced; a book with no
priced closes reports nulls, never a fabricated zero.
*/
export const summarizeBook = (
	rows: Record<string, unknown>[],
	book: Book,
): BookDeskStats => {
	const onThisBook = rows.filter((row) => onBook(row, book));
	const closed = closedOnBook(rows, book);
	const returns = closed
		.map((row) => positionReadout(row).returnPct)
		.filter((value): value is number => value !== null);
	const wins = returns.filter((value) => value > 0).length;
	const holds = closed
		.map((row) => positionReadout(row).holdNs)
		.filter((value): value is number => value !== null && value > 0);

	return {
		closed: closed.length,
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
summarizeTraining aggregates the live grammar feeds into one TrainingSample.
Rates are null when their denominator is zero: a feed that has not reported
a gate is not a gate that passes 0%.
*/
export const summarizeTraining = (feeds: SequiturFeed[]): TrainingSample => {
	let feedsWithFrame = 0;
	let gateTotal = 0;
	let gatePass = 0;
	let warm = 0;
	let blocked = 0;
	let surpriseSum = 0;
	let surpriseN = 0;
	let zSum = 0;
	let zN = 0;
	let rules = 0;
	let grammarSymbols = 0;
	let rawLength = 0;
	let depthSum = 0;
	let depthN = 0;
	let maxDepth = 0;
	let actionRules = 0;
	let eventTotal = 0;
	let eventCreate = 0;
	let eventReuse = 0;
	let eventInline = 0;
	let policyTotal = 0;
	let stored = 0;
	let matched = 0;
	let matchDepthSum = 0;
	let matchDepthN = 0;
	let outcomes = 0;
	let violations = 0;

	for (const feed of feeds) {
		const frame = feed.latest;
		if (!frame) continue;
		feedsWithFrame += 1;

		if (frame.gate) {
			gateTotal += 1;
			if (frame.gate.passed) gatePass += 1;
			if (frame.gate.warm) warm += 1;
			if (Number.isFinite(frame.gate.surprise)) {
				surpriseSum += frame.gate.surprise;
				surpriseN += 1;
			}
			if (frame.gate.z !== null && Number.isFinite(frame.gate.z)) {
				zSum += frame.gate.z;
				zN += 1;
			}
		}
		if (frame.blocked) blocked += 1;

		for (const rule of frame.rules) {
			rules += 1;
			depthSum += rule.depth;
			depthN += 1;
			if (rule.depth > maxDepth) maxDepth = rule.depth;
			if (rule.action) actionRules += 1;
		}
		grammarSymbols += frame.grammarSymbols;
		rawLength += frame.rawLength;

		for (const event of frame.events) {
			eventTotal += 1;
			if (event.kind === "create") eventCreate += 1;
			else if (event.kind === "reuse") eventReuse += 1;
			else if (event.kind === "inline") eventInline += 1;
		}

		policyTotal += 1;
		if (frame.policy.stored) stored += 1;
		if (frame.policy.match !== "") {
			matched += 1;
			matchDepthSum += frame.policy.depth;
			matchDepthN += 1;
		}
		if (frame.policy.outcome !== "") outcomes += 1;

		violations += frame.violations.length;
	}

	const rate = (numerator: number, denominator: number): number | null =>
		denominator > 0 ? numerator / denominator : null;

	return {
		feeds: feedsWithFrame,
		rules: feedsWithFrame > 0 ? rules : null,
		grammarSymbols: feedsWithFrame > 0 ? grammarSymbols : null,
		rawLength: feedsWithFrame > 0 ? rawLength : null,
		avgRuleDepth: rate(depthSum, depthN),
		maxRuleDepth: depthN > 0 ? maxDepth : null,
		actionCoverage: rate(actionRules, rules),
		createRate: rate(eventCreate, eventTotal),
		reuseRate: rate(eventReuse, eventTotal),
		inlineRate: rate(eventInline, eventTotal),
		gatePassRate: rate(gatePass, gateTotal),
		blockedRate: rate(blocked, policyTotal),
		warmRate: rate(warm, gateTotal),
		avgSurprise: rate(surpriseSum, surpriseN),
		avgZ: rate(zSum, zN),
		policyStoredRate: rate(stored, policyTotal),
		policyMatchRate: rate(matched, policyTotal),
		avgMatchDepth: rate(matchDepthSum, matchDepthN),
		outcomeRate: rate(outcomes, policyTotal),
		violations: feedsWithFrame > 0 ? violations : null,
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

	if (Math.abs(current - prior) < 0.01) {
		return "flat";
	}

	return current > prior ? "up" : "down";
};

/*
verdict answers the question the surface exists for — is the system doing
well? — from the latest sample's levels and the edge's trend over the recent
window. It never reports improving from a bad level, and never reports a
verdict from data it does not have.
*/
export const verdict = (samples: LongtailSample[], book: Book): Verdict => {
	if (samples.length < 2) {
		return { label: "Warming up", tone: "neutral" };
	}

	const latest = samples[samples.length - 1];
	const prior = samples[Math.max(0, samples.length - 16)];

	const levels = [
		returnTone(latest[book].meanReturn),
		winRateTone(latest[book].winRate),
		reuseTone(latest.training.reuseRate),
		matchTone(latest.training.policyMatchRate),
		gatePassTone(latest.training.gatePassRate),
	].filter((tone) => tone !== "neutral");

	if (levels.length < 2) {
		return { label: "Warming up", tone: "neutral" };
	}

	const good = levels.filter((tone) => tone === "good").length;
	const bad = levels.filter((tone) => tone === "bad").length;
	const trend = trendOf(latest[book].meanReturn, prior[book].meanReturn);

	if (bad >= 2) {
		return trend === "up"
			? { label: "Recovering", tone: "warn" }
			: { label: "Degrading", tone: "bad" };
	}
	if (good >= 3) {
		if (trend === "down") return { label: "Slipping", tone: "warn" };
		if (trend === "up") return { label: "Improving", tone: "good" };
		return { label: "Stable", tone: "info" };
	}
	if (good >= 2) {
		return { label: "Stable", tone: "info" };
	}
	return { label: "Mixed", tone: "warn" };
};

/*
buildInsights writes the plain-language readings the right rail shows. Each
insight carries the tone of the metric it describes, so a bad number is never
announced in green.
*/
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
				body: `Samples arrive from ${book} lots and the live grammar feeds. Nothing to read yet.`,
				tone: "neutral",
			},
		];
	}

	const prior = samples[Math.max(0, samples.length - 16)];
	const bookStats = latest[book];
	const training = latest.training;
	const insights: Insight[] = [];

	if (bookStats.winRate === null) {
		insights.push({
			id: "edge",
			title: `Round-trip edge (${book})`,
			body: `No priced ${book} round trips yet — edge stays unknown until a lot settles with costs.`,
			tone: "neutral",
		});
	} else {
		const trend = trendOf(bookStats.meanReturn, prior[book].meanReturn);
		const trendSentence =
			trend === "up"
				? " Edge is improving versus the earlier window."
				: trend === "down"
					? " Edge is slipping versus the earlier window."
					: " Edge is steady on this horizon.";
		insights.push({
			id: "edge",
			title: `Round-trip edge (${book})`,
			body: `Mean return ${bookStats.meanReturn === null ? "—" : signedPct(bookStats.meanReturn)} on ${bookStats.closed} closed ${book} lots; win rate ${pct(bookStats.winRate)} (${bookStats.wins}/${bookStats.closed}).${trendSentence}`,
			tone: returnTone(bookStats.meanReturn),
		});
	}

	if (training.rules === null) {
		insights.push({
			id: "grammar",
			title: "Grammar",
			body: "No grammar rules yet — the sequitur has not found repeating structure.",
			tone: "neutral",
		});
	} else {
		const reuse = training.reuseRate;
		const create = training.createRate;
		const structureSentence =
			reuse === null || create === null
				? ""
				: reuseTone(reuse) === "good"
					? " Structure is generalizing."
					: reuseTone(reuse) === "bad"
						? " It is still mostly inventing new rules — it has not found repeating structure."
						: " Reuse and creation are balanced.";
		insights.push({
			id: "grammar",
			title: "Grammar",
			body: `${training.rules} rules (avg depth ${fixed(training.avgRuleDepth ?? 0, 1)}, max ${training.maxRuleDepth ?? 0}); ${reuse === null ? "—" : pct(reuse)} of recent events reuse existing rules, ${create === null ? "—" : pct(create)} create new ones.${structureSentence}`,
			tone: reuseTone(reuse),
		});
	}

	if (training.gatePassRate === null) {
		insights.push({
			id: "gate",
			title: "Surprise gate",
			body: "No surprise-gate readings yet.",
			tone: "neutral",
		});
	} else {
		insights.push({
			id: "gate",
			title: "Surprise gate",
			body: `Passes ${pct(training.gatePassRate)} of chords; mean surprise ${fixed(training.avgSurprise ?? 0)}, coders warm on ${training.warmRate === null ? "—" : pct(training.warmRate)} of feeds.`,
			tone: gatePassTone(training.gatePassRate),
		});
	}

	if (training.policyStoredRate === null) {
		insights.push({
			id: "policy",
			title: "Policy",
			body: "No policy snapshots on the wire yet.",
			tone: "neutral",
		});
	} else {
		insights.push({
			id: "policy",
			title: "Policy",
			body: `Stored paths match on ${training.policyMatchRate === null ? "—" : pct(training.policyMatchRate)} of frames (avg depth ${fixed(training.avgMatchDepth ?? 0, 1)}); the desk acted on ${training.outcomeRate === null ? "—" : pct(training.outcomeRate)}.`,
			tone: matchTone(training.policyMatchRate),
		});
	}

	if ((training.violations ?? 0) > 0) {
		insights.push({
			id: "violations",
			title: "Grammar invariants",
			body: `${training.violations} invariant violation(s) in the latest frames.`,
			tone: "bad",
		});
	}

	return insights;
};

export const longtailStore = createStore({
	samples: [] as LongtailSample[],
	lastSampleAt: 0,
});

/*
One hour of 5-second samples: long enough to see a trend, small enough to
keep in memory. The recorder runs for the whole session (started from the
feed), so the window keeps filling on every surface, not only this one.
*/
const MAX_SAMPLES = 720;
const SAMPLE_EVERY_MS = 5_000;

const pushSample = (sample: LongtailSample) => {
	longtailStore.setState((state) => {
		const samples = [...state.samples, sample].slice(-MAX_SAMPLES);
		return {
			samples,
			lastSampleAt: sample.at,
		};
	});
};

const readPositions = () => positionRows(signals.positions.state);

let recorderStarted = false;
let recorderHandle: number | null = null;

export const ensureLongtailRecorder = () => {
	if (recorderStarted) {
		return;
	}

	recorderStarted = true;

	const tick = () => {
		const desk = readPositions();
		ingestDeskClosedLots(desk);

		pushSample({
			at: Date.now(),
			paper: summarizeBook(desk, "paper"),
			virtual: summarizeBook(desk, "virtual"),
			training: summarizeTraining(Object.values(sequiturStore.state)),
		});
	};

	tick();
	recorderHandle = window.setInterval(tick, SAMPLE_EVERY_MS);
};

/* Tests: stop the interval and empty the window. */
export const resetLongtailRecorder = () => {
	if (recorderHandle !== null) {
		window.clearInterval(recorderHandle);
		recorderHandle = null;
	}
	recorderStarted = false;
	longtailStore.setState({ samples: [], lastSampleAt: 0 });
};

export const useLongtailReport = () =>
	useSelector(longtailStore, (state) => state);

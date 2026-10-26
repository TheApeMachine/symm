import { useSelector } from "@tanstack/react-store";
import { createStore } from "@tanstack/store";
import type { Book } from "#/collections/book";
import { Circular, type CircularBuffer } from "#/collections/circular";
import { type SequiturFeed, sequiturStore } from "#/collections/sequitur";
import {
	closedOnBook,
	countsTowardBookPerformance,
	onBook,
	openOnBook,
	positionReadout,
} from "#/components/book-readout";
import { deskRowsForStats, ingestDeskClosedLots } from "#/lib/lifecycle";

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
	tone === "good"
		? "success"
		: tone === "warn"
			? "warning"
			: tone === "bad"
				? "error"
				: "info";

/*
Per-metric tone thresholds. A null reading is neutral: absence of data is
neither good nor bad, and painting it green or red would invent a verdict.
*/
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

/** Token emit rate: how often lit ticks become path edges. */
export const tokenEmitTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value >= 0.15
			? "good"
			: value >= 0.05
				? "warn"
				: "bad";

/** Blocked rate: high means grid lit but tokenizer/path stayed quiet. */
export const blockedTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value <= 0.7
			? "good"
			: value <= 0.9
				? "warn"
				: "bad";

/** @deprecated use tokenEmitTone */
export const gatePassTone = tokenEmitTone;

/*
Surprise is the absolute energy error against the running prediction.
Lower is better. A negative value is not a better prediction: the error
has no sign, so a negative reading is a broken measurement.
*/
export const surpriseTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value < 0
			? "bad"
			: value <= 0.5
				? "good"
				: value <= 2
					? "warn"
					: "bad";

/* A maturing grammar reuses its rules; one that mostly creates new rules has not found structure. */
export const reuseTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value >= 0.5
			? "good"
			: value >= 0.25
				? "warn"
				: "bad";

/* The share of grammar rules that carry a stored enter/exit action: how much of the grammar is trained. */
export const coverageTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value >= 0.5
			? "good"
			: value >= 0.25
				? "warn"
				: "bad";

/* How often the live token path ends on a stored policy path. */
export const matchTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value >= 0.5
			? "good"
			: value >= 0.2
				? "warn"
				: "bad";

export const pathStableTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value >= 0.85
			? "good"
			: value >= 0.6
				? "warn"
				: "bad";

export const frictionPassTone = gatePassTone;

/* Coders whose error has settled: the gate can only trust warm symbols. */
export const warmTone = (value: number | null): Tone =>
	value === null
		? "neutral"
		: value >= 0.7
			? "good"
			: value >= 0.4
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
One snapshot of the training system, aggregated across every symbol's live
grammar feed: the grammar's structure, the mix of structural events its last
append caused, the surprise gate's readings, and how the stored policy met the
live token path.
*/
export type BackendTickMetrics = {
	symbols: number | null;
	gatePass: number | null;
	gateTotal: number | null;
	litTicks: number | null;
	blocked: number | null;
	blockedBrightness: number | null;
	blockedDuplicate: number | null;
	blockedVeto: number | null;
	blockedEmpty: number | null;
	policyChecks: number | null;
	policyStored: number | null;
	outcomeEnter: number | null;
	outcomeExit: number | null;
	exploredEnter: number | null;
	venueOpen: number | null;
	virtualExcursion: number | null;
	developedPaths: number | null;
	promotedPaths: number | null;
	prunedPaths: number | null;
	meanThreshold: number | null;
	exitProfit: number | null;
	exitStop: number | null;
	exitBranch: number | null;
};

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
	/** Share of symbol steps whose regime.posture letter changed. */
	tokenEmitRate: number | null;
	/** Lit ticks that did not append a new letter. */
	blockedRate: number | null;
	blockedBrightnessRate: number | null;
	/** Same letter as the previous step. The story did not change. */
	blockedDuplicateRate: number | null;
	/** Lit ticks where risk brightness beat the posture. */
	vetoRate: number | null;
	/** Lit ticks where a role band was dark. */
	emptyRate: number | null;
	policyStoredRate: number | null;
	outcomeRate: number | null;
	outcomeEnterRate: number | null;
	outcomeExitRate: number | null;
	violations: number | null;
	exploredEnter: number | null;
	venueOpen: number | null;
	virtualExcursion: number | null;
	developedPaths: number | null;
	promotedPaths: number | null;
	prunedPaths: number | null;
	meanThreshold: number | null;
	exitProfit: number | null;
	exitStop: number | null;
	exitBranch: number | null;
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
	values.length > 0
		? values.reduce((sum, value) => sum + value, 0) / values.length
		: null;

/*
summarizeBook reads one book's desk rows into round-trip statistics via
positionReadout (broker pnl / CloseOut economics). Win rate and edge count
only lots that reused an existing path. A virtual discovery is opened on
every symbol, so it stays out of the comparison. Open lots never affect
edge. Nulls when nothing priced has reused a path — never a fabricated zero.
*/
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
	const returns = scored.map(
		(row) => positionReadout(row).returnPct as number,
	);
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
summarizeTraining aggregates the live grammar feeds into one TrainingSample.
Rates are null when their denominator is zero: a feed that has not reported
a gate is not a gate that passes 0%.
*/
export const summarizeTraining = (feeds: SequiturFeed[]): TrainingSample => {
	let feedsWithFrame = 0;
	let gateTotal = 0;
	let gatePass = 0;
	let blocked = 0;
	let blockedBrightness = 0;
	let blockedDuplicate = 0;
	let blockedVeto = 0;
	let blockedEmpty = 0;
	let litTicks = 0;
	let outcomeEnter = 0;
	let outcomeExit = 0;
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
	let outcomes = 0;
	let violations = 0;

	for (const feed of feeds) {
		const frame = feed.latest;
		if (!frame) continue;
		feedsWithFrame += 1;

		if (frame.active.length > 0) {
			litTicks += 1;
		}
		if (frame.gate) {
			gateTotal += 1;
			if (frame.gate.passed) gatePass += 1;
		}
		if (frame.blocked) {
			blocked += 1;
			if (frame.blockedReason === "brightness") blockedBrightness += 1;
			if (frame.blockedReason === "duplicate") blockedDuplicate += 1;
			if (frame.blockedReason === "veto") blockedVeto += 1;
			if (frame.blockedReason === "empty") blockedEmpty += 1;
		}

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
		if (frame.policy.outcome !== "") {
			outcomes += 1;
			if (frame.policy.outcome.includes("enter")) outcomeEnter += 1;
			if (frame.policy.outcome.includes("exit")) outcomeExit += 1;
		}

		violations += frame.violations.length;
	}

	const rate = (numerator: number, denominator: number): number | null =>
		denominator > 0 ? numerator / denominator : null;

	const backend = longtailBackendMetrics;
	const backendRate = (
		numerator: number | null | undefined,
		denominator: number | null | undefined,
		fallbackNumerator: number,
		fallbackDenominator: number,
	): number | null => {
		if (backend) {
			if (numerator === null || numerator === undefined) return null;
			return rate(numerator, denominator ?? 0);
		}
		return rate(fallbackNumerator, fallbackDenominator);
	};

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
		tokenEmitRate: backendRate(
			backend?.gatePass,
			backend?.gateTotal,
			gatePass,
			gateTotal,
		),
		blockedRate: backendRate(
			backend?.blocked,
			backend?.litTicks,
			blocked,
			litTicks,
		),
		blockedBrightnessRate: backendRate(
			backend?.blockedBrightness,
			backend?.litTicks,
			blockedBrightness,
			litTicks,
		),
		blockedDuplicateRate: backendRate(
			backend?.blockedDuplicate,
			backend?.gateTotal,
			blockedDuplicate,
			gateTotal,
		),
		vetoRate: backendRate(
			backend?.blockedVeto,
			backend?.litTicks,
			blockedVeto,
			litTicks,
		),
		emptyRate: backendRate(
			backend?.blockedEmpty,
			backend?.litTicks,
			blockedEmpty,
			litTicks,
		),
		policyStoredRate: backendRate(
			backend?.policyStored,
			backend?.policyChecks,
			stored,
			policyTotal,
		),
		outcomeRate: rate(outcomes, policyTotal),
		outcomeEnterRate: backendRate(
			backend?.outcomeEnter,
			backend?.gateTotal,
			outcomeEnter,
			policyTotal,
		),
		outcomeExitRate: backendRate(
			backend?.outcomeExit,
			backend?.gateTotal,
			outcomeExit,
			policyTotal,
		),
		violations: feedsWithFrame > 0 ? violations : null,
		exploredEnter: backend ? backend.exploredEnter : null,
		venueOpen: backend ? backend.venueOpen : null,
		virtualExcursion: backend ? backend.virtualExcursion : null,
		developedPaths: backend ? backend.developedPaths : null,
		promotedPaths: backend ? backend.promotedPaths : null,
		prunedPaths: backend ? backend.prunedPaths : null,
		meanThreshold:
			backend && (backend.developedPaths ?? 0) > 0
				? backend.meanThreshold
				: null,
		exitProfit: backend ? backend.exitProfit : null,
		exitStop: backend ? backend.exitStop : null,
		exitBranch: backend ? backend.exitBranch : null,
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
verdict answers whether settled lots are making money, and whether that
is changing. It reads the book's mean return and win rate only. A rising
return that is still negative is Recovering. It never calls a losing book
Improving.
*/
export const verdict = (samples: LongtailSample[], book: Book): Verdict => {
	if (samples.length < 2) {
		return { label: "Warming up", tone: "neutral" };
	}

	const latest = samples[samples.length - 1];
	const prior = samples[Math.max(0, samples.length - 16)];
	const edge = returnTone(latest[book].meanReturn);
	const wins = winRateTone(latest[book].winRate);

	if (edge === "neutral" || wins === "neutral") {
		return { label: "Warming up", tone: "neutral" };
	}

	const trend = trendOf(latest[book].meanReturn, prior[book].meanReturn);
	const losing = edge === "bad" || wins === "bad";
	const winning = edge === "good" && wins !== "bad";

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

/*
trainingVerdict reads grammar reuse, policy hits, and path-book growth —
not desk PnL (that lives in the top bar).
*/
export const trainingVerdict = (samples: LongtailSample[]): Verdict => {
	if (samples.length < 2) {
		return { label: "Warming up", tone: "neutral" };
	}

	const latest = samples[samples.length - 1].training;
	const prior = samples[Math.max(0, samples.length - 16)].training;

	if (
		latest.reuseRate === null &&
		latest.policyStoredRate === null &&
		latest.developedPaths === null
	) {
		return { label: "Warming up", tone: "neutral" };
	}

	const reuse = latest.reuseRate ?? 0;
	const create = latest.createRate ?? 0;
	const policy = latest.policyStoredRate ?? 0;
	const blocked = latest.blockedRate ?? 0;
	const pathTrend = trendOf(latest.developedPaths, prior.developedPaths);

	if (reuse >= 0.5 && policy >= 0.25) {
		if (pathTrend === "up") return { label: "Reusing paths", tone: "good" };
		return { label: "Reusing paths", tone: "info" };
	}

	if (create >= 0.55 && reuse < 0.3) {
		return { label: "Finding structure", tone: "info" };
	}

	if (blocked >= 0.85 && (latest.tokenEmitRate ?? 0) < 0.05) {
		return { label: "Quiet gate", tone: "warn" };
	}

	if ((latest.developedPaths ?? 0) > 0 && pathTrend === "up") {
		return { label: "Path book growing", tone: "good" };
	}

	return { label: "Learning", tone: "neutral" };
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
				body: `Samples arrive from settled ${book} lots and the path book. Nothing to read yet.`,
				tone: "neutral",
			},
		];
	}

	const prior = samples[Math.max(0, samples.length - 16)];
	const training = latest.training;
	const insights: Insight[] = [];

	if (training.reuseRate === null && training.createRate === null) {
		insights.push({
			id: "grammar",
			title: "Grammar structure",
			body: "No sequitur event mix yet. Reuse means the spelling already existed; create means a new rule was minted.",
			tone: "neutral",
		});
	} else {
		const reuseTrend = trendOf(training.reuseRate, prior.training.reuseRate);
		const trendSentence =
			reuseTrend === "up"
				? " Reuse is rising versus the earlier window."
				: reuseTrend === "down"
					? " Reuse is falling — more new rules than before."
					: "";
		const coverage =
			training.actionCoverage === null
				? "—"
				: pct(training.actionCoverage);
		insights.push({
			id: "grammar",
			title: "Grammar structure",
			body: `Rule events: ${pct(training.reuseRate ?? 0)} reuse, ${pct(training.createRate ?? 0)} create, ${pct(training.inlineRate ?? 0)} inline. ${training.rules ?? 0} rules; ${coverage} carry a stored enter/exit.${trendSentence}`,
			tone: reuseTone(training.reuseRate),
		});
	}

	if (training.policyStoredRate === null && training.outcomeRate === null) {
		insights.push({
			id: "policy",
			title: "Policy match",
			body: "No policy checks yet. A hit means the live token path ended on a stored explore or policy leaf.",
			tone: "neutral",
		});
	} else {
		insights.push({
			id: "policy",
			title: "Policy match",
			body: `${pct(training.policyStoredRate ?? 0)} of symbol checks hit a stored path; ${pct(training.outcomeRate ?? 0)} fired an enter/exit outcome (${pct(training.outcomeEnterRate ?? 0)} enter, ${pct(training.outcomeExitRate ?? 0)} exit).`,
			tone: matchTone(training.policyStoredRate),
		});
	}

	if (training.developedPaths === null) {
		insights.push({
			id: "paths",
			title: "Paths",
			body: "The path book has not reported on this sample. Counts appear once a tick publishes developed, paper, and pruned paths.",
			tone: "neutral",
		});
	} else {
		const threshold =
			training.meanThreshold === null
				? "—"
				: training.meanThreshold.toFixed(2);
		insights.push({
			id: "paths",
			title: "Paths",
			body: `${training.developedPaths} developed, ${training.promotedPaths ?? 0} at the paper floor, ${training.prunedPaths ?? 0} pruned. Mean threshold ${threshold} (starts at 0.50; profit lowers it, loss raises it).`,
			tone: (training.promotedPaths ?? 0) > 0 ? "good" : "info",
		});
	}

	if (training.tokenEmitRate === null) {
		insights.push({
			id: "emit",
			title: "Path letters",
			body: "No letter readings yet. A letter is the brightest regime and posture, appended only when it changes.",
			tone: "neutral",
		});
	} else {
		const parts = [
			training.blockedDuplicateRate === null
				? null
				: `${pct(training.blockedDuplicateRate)} repeated the same letter`,
			training.vetoRate === null
				? null
				: `${pct(training.vetoRate)} of lit ticks vetoed by risk`,
			training.emptyRate === null
				? null
				: `${pct(training.emptyRate)} of lit ticks had a dark role`,
		].filter(Boolean);
		insights.push({
			id: "emit",
			title: "Path letters",
			body: `A new letter on ${pct(training.tokenEmitRate)} of symbol steps${parts.length ? `. ${parts.join("; ")}.` : "."}`,
			tone: "info",
		});
	}

	const exits =
		(training.exitProfit ?? 0) +
		(training.exitStop ?? 0) +
		(training.exitBranch ?? 0);
	if (training.exitProfit === null) {
		insights.push({
			id: "exits",
			title: "Exits",
			body: "No exit ledger yet. Discovery closes on book profit or the friction stop. A reused path closes on its learned exit branch.",
			tone: "neutral",
		});
	} else if (exits === 0) {
		insights.push({
			id: "exits",
			title: "Exits",
			body: "No virtuals have closed through profit, the friction stop, or a learned exit branch.",
			tone: "neutral",
		});
	} else {
		insights.push({
			id: "exits",
			title: "Exits",
			body: `${training.exitProfit} cleared the spread, ${training.exitStop} hit the friction stop, ${training.exitBranch} followed a learned exit branch.`,
			tone: (training.exitProfit ?? 0) > (training.exitStop ?? 0) ? "good" : "warn",
		});
	}

	if (
		training.virtualExcursion !== null &&
		training.virtualExcursion > 0
	) {
		insights.push({
			id: "ledger",
			title: "Desk",
			body: `${training.virtualExcursion} virtual excursion(s) open; ${training.venueOpen ?? 0} venue lot(s); ${training.exploredEnter ?? 0} explore enter(s) this tick window.`,
			tone: "info",
		});
	}

	return insights;
};

export let longtailBackendMetrics: BackendTickMetrics | null = null;

export const ingestLongtailTick = (payload: unknown) => {
	if (!payload || typeof payload !== "object") {
		return false;
	}

	const body = payload as Record<string, unknown>;
	const read = (key: string): number | null => {
		const value = body[key];
		return typeof value === "number" && Number.isFinite(value) ? value : null;
	};

	longtailBackendMetrics = {
		symbols: read("symbols"),
		gatePass: read("gatePass"),
		gateTotal: read("gateTotal"),
		litTicks: read("litTicks"),
		blocked: read("blocked"),
		blockedBrightness: read("blockedBrightness"),
		blockedDuplicate: read("blockedDuplicate"),
		blockedVeto: read("blockedVeto"),
		blockedEmpty: read("blockedEmpty"),
		policyChecks: read("policyChecks"),
		policyStored: read("policyStored"),
		outcomeEnter: read("outcomeEnter"),
		outcomeExit: read("outcomeExit"),
		exploredEnter: read("exploredEnter"),
		venueOpen: read("venueOpen"),
		virtualExcursion: read("virtualExcursion"),
		developedPaths: read("developedPaths"),
		promotedPaths: read("promotedPaths"),
		prunedPaths: read("prunedPaths"),
		meanThreshold: read("meanThreshold"),
		exitProfit: read("exitProfit"),
		exitStop: read("exitStop"),
		exitBranch: read("exitBranch"),
	};

	maybeRecordLongtailSample();
	return true;
};

export const longtailStore = createStore({
	samples: [] as LongtailSample[],
	lastSampleAt: 0,
});

/*
One sample every five seconds. Two hundred and forty of them fill the
charts with twenty minutes, and then the line scrolls.
*/
export const CHART_SAMPLES = 240;
const SAMPLE_EVERY_MS = 5_000;

let chartWindow = Circular<LongtailSample>(CHART_SAMPLES);

const publishWindow = (lastSampleAt?: number) => {
	const samples = chartWindow.values();
	longtailStore.setState(() => ({
		samples,
		lastSampleAt: lastSampleAt ?? samples[samples.length - 1]?.at ?? 0,
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

const readPositions = () => deskRowsForStats();

const recordLongtailSample = () => {
	const desk = readPositions();
	ingestDeskClosedLots(desk);

	pushSample({
		at: Date.now(),
		paper: summarizeBook(desk, "paper"),
		virtual: summarizeBook(desk, "virtual"),
		training: summarizeTraining(Object.values(sequiturStore.state)),
	});
};

const maybeRecordLongtailSample = () => {
	const { lastSampleAt, samples } = longtailStore.state;
	const now = Date.now();
	if (
		samples.length > 0 &&
		now - lastSampleAt < SAMPLE_EVERY_MS
	) {
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

const trainingFromHistory = (value: unknown): TrainingSample | null => {
	if (!value || typeof value !== "object") return null;

	const row = value as Record<string, unknown>;

	return {
		feeds: countOrZero(row.feeds),
		rules: finiteOrNull(row.rules),
		grammarSymbols: finiteOrNull(row.grammarSymbols),
		rawLength: finiteOrNull(row.rawLength),
		avgRuleDepth: finiteOrNull(row.avgRuleDepth),
		maxRuleDepth: finiteOrNull(row.maxRuleDepth),
		actionCoverage: finiteOrNull(row.actionCoverage),
		createRate: finiteOrNull(row.createRate),
		reuseRate: finiteOrNull(row.reuseRate),
		inlineRate: finiteOrNull(row.inlineRate),
		tokenEmitRate: finiteOrNull(row.tokenEmitRate ?? row.gatePassRate),
		blockedRate: finiteOrNull(row.blockedRate),
		blockedBrightnessRate: finiteOrNull(row.blockedBrightnessRate),
		blockedDuplicateRate: finiteOrNull(row.blockedDuplicateRate),
		vetoRate: finiteOrNull(row.vetoRate),
		emptyRate: finiteOrNull(row.emptyRate),
		policyStoredRate: finiteOrNull(row.policyStoredRate),
		outcomeRate: finiteOrNull(row.outcomeRate),
		outcomeEnterRate: finiteOrNull(row.outcomeEnterRate),
		outcomeExitRate: finiteOrNull(row.outcomeExitRate),
		violations: finiteOrNull(row.violations),
		exploredEnter: finiteOrNull(row.exploredEnter),
		venueOpen: finiteOrNull(row.venueOpen),
		virtualExcursion: finiteOrNull(row.virtualExcursion),
		developedPaths: finiteOrNull(row.developedPaths),
		promotedPaths: finiteOrNull(row.promotedPaths),
		prunedPaths: finiteOrNull(row.prunedPaths),
		meanThreshold: finiteOrNull(row.meanThreshold),
		exitProfit: finiteOrNull(row.exitProfit),
		exitStop: finiteOrNull(row.exitStop),
		exitBranch: finiteOrNull(row.exitBranch),
	};
};

const historySample = (value: unknown): LongtailSample | null => {
	if (!value || typeof value !== "object") return null;

	const row = value as Record<string, unknown>;
	const at = finiteOrNull(row.at);
	const paper = bookFromHistory(row.paper);
	const virtual = bookFromHistory(row.virtual);
	const training = trainingFromHistory(row.training);

	if (at === null || paper === null || virtual === null || training === null) {
		return null;
	}

	return { at, paper, virtual, training };
};

/*
beginLongtailHistory opens the buffer for one replay. The samples already
on the page stay up until the replay ends, then the epoch replaces them.
*/
export const beginLongtailHistory = () => {
	historyBuffer = Circular<LongtailSample>(CHART_SAMPLES);
};

/* ingestLongtailHistory appends one batch. A batch before begin is ignored. */
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

/*
endLongtailHistory installs the epoch. An empty replay leaves the page's
own samples alone. A recorder that is already running then takes one live
reading, so the latest point still carries the grammar feed.
*/
export const endLongtailHistory = () => {
	if (historyBuffer === null) return;

	const seeded = historyBuffer.values().sort((a, b) => a.at - b.at);
	historyBuffer = null;

	if (seeded.length === 0) return;

	installWindow(seeded);
	maybeRecordLongtailSample();
};

export const ensureLongtailRecorder = () => {
	if (recorderHandle !== null) {
		return;
	}

	recordLongtailSample();
	recorderHandle = window.setInterval(
		maybeRecordLongtailSample,
		SAMPLE_EVERY_MS,
	);
};

/* Tests: stop the interval and empty the window. */
export const resetLongtailRecorder = () => {
	if (recorderHandle !== null) {
		window.clearInterval(recorderHandle);
		recorderHandle = null;
	}
	historyBuffer = null;
	longtailBackendMetrics = null;
	chartWindow = Circular<LongtailSample>(CHART_SAMPLES);
	longtailStore.setState(() => ({ samples: [], lastSampleAt: 0 }));
};

export const useLongtailReport = () =>
	useSelector(longtailStore, (state) => state);

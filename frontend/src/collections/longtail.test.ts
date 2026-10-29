import { afterEach, describe, expect, it } from "vitest";
import {
	alertVariant,
	type BookDeskStats,
	beginLongtailHistory,
	buildInsights,
	coverageTone,
	endLongtailHistory,
	blockedTone,
	tokenEmitTone,
	CHART_SAMPLES,
	ingestLongtailHistory,
	ingestLongtailTick,
	type LongtailSample,
	type OpenLotTelemetry,
	longtailStore,
	matchTone,
	resetLongtailRecorder,
	returnTone,
	reuseTone,
	summarizeBook,
	summarizeTraining,
	surpriseTone,
	type TrainingSample,
	toneColor,
	toneVariant,
	trainingVerdict,
	verdict,
	warmTone,
	winRateTone,
} from "./longtail";
import type { SequiturFeed } from "./sequitur";
import { type SequiturFrame, sequiturFrame } from "./sequitur-fixture";

afterEach(() => {
	resetLongtailRecorder();
});

/* One live grammar feed around a frame, shaped as the store holds it. */
const feed = (frame: SequiturFrame): SequiturFeed => ({
	latest: frame,
	frames: 1,
	events: frame.events.map((event, i) => ({
		...event,
		tick: frame.tick,
		seq: i,
	})),
	chords: [],
	receivedAt: 0,
});

/* A settled desk lot: closed, priced, with a net return in percent. */
const closedLot = (
	symbol: string,
	virtual: boolean,
	netReturnPct: number,
	holdNs = 60_000_000_000,
	pathFound = virtual,
) => ({
	id: `${symbol}-${virtual}-${netReturnPct}`,
	symbol,
	virtual,
	path_found: virtual ? pathFound : undefined,
	closed_at: "2026-01-01T00:00:00Z",
	pnl: netReturnPct / 100,
	pnl_pct: netReturnPct,
	hold_ns: holdNs,
});

const openLot = (symbol: string, virtual: boolean) => ({
	id: `open-${symbol}-${virtual}`,
	symbol,
	virtual,
	opened_at: "2026-01-01T00:00:00Z",
	mark: 100,
});

describe("summarizeBook", () => {
	it("reports nulls, never fabricated zeros, for a book with no priced closes", () => {
		const stats = summarizeBook([openLot("BTC/USD", true)], "virtual");

		expect(stats.closed).toBe(0);
		expect(stats.open).toBe(1);
		expect(stats.winRate).toBeNull();
		expect(stats.meanReturn).toBeNull();
		expect(stats.medianReturn).toBeNull();
		expect(stats.worstReturn).toBeNull();
	});

	it("counts a paper lot when the desk omits the virtual flag", () => {
		const paper = openLot("BTC/USD", false);
		delete paper.virtual;

		expect(summarizeBook([paper], "paper").open).toBe(1);
		expect(summarizeBook([paper], "virtual").open).toBe(0);
	});

	it("ignores closed lots the desk has not priced for edge stats", () => {
		const rows = [
			{
				id: "pending",
				symbol: "BTC/USD",
				virtual: false,
				closed_at: "2026-01-01T00:00:00Z",
			},
			openLot("ETH/USD", true),
		];

		const stats = summarizeBook(rows, "virtual");
		expect(stats.closed).toBe(0);
		expect(stats.winRate).toBeNull();
		expect(stats.open).toBe(1);
	});

	it("keeps virtual discovery closes out of win rate and edge", () => {
		const rows = [
			closedLot("BTC/USD", true, 50, 60_000_000_000, false),
			closedLot("ETH/USD", true, -10, 60_000_000_000, true),
		];

		const virtual = summarizeBook(rows, "virtual");
		expect(virtual.closed).toBe(1);
		expect(virtual.wins).toBe(0);
		expect(virtual.winRate).toBeCloseTo(0);
		expect(virtual.meanReturn).toBeCloseTo(-0.1);
		expect(virtual.avgHoldNs).toBe(60_000_000_000);
	});

	it("summarizes settled round trips per book", () => {
		const rows = [
			closedLot("BTC/USD", true, 5),
			closedLot("ETH/USD", true, -10),
			closedLot("SOL/USD", true, 2),
			closedLot("BTC/USD", false, 1),
			openLot("DOGE/USD", true),
		];

		const virtual = summarizeBook(rows, "virtual");
		expect(virtual.closed).toBe(3);
		expect(virtual.open).toBe(1);
		expect(virtual.wins).toBe(2);
		expect(virtual.winRate).toBeCloseTo(2 / 3);
		expect(virtual.meanReturn).toBeCloseTo((5 - 10 + 2) / 3 / 100);
		expect(virtual.medianReturn).toBeCloseTo(0.02);
		expect(virtual.worstReturn).toBeCloseTo(-0.1);
		expect(virtual.avgHoldNs).toBe(60_000_000_000);

		const paper = summarizeBook(rows, "paper");
		expect(paper.closed).toBe(1);
		expect(paper.winRate).toBe(1);
	});
});

describe("summarizeTraining", () => {
	it("reports nulls for a feed that has not reported", () => {
		const sample = summarizeTraining([]);

		expect(sample.feeds).toBe(0);
		expect(sample.rules).toBeNull();
		expect(sample.tokenEmitRate).toBeNull();
		expect(sample.reuseRate).toBeNull();
		expect(sample.policyStoredRate).toBeNull();
	});

	it("aggregates the live grammar feeds", () => {
		const sample = summarizeTraining([
			feed(sequiturFrame()),
			feed(
				sequiturFrame({
					symbol: "ETH/USD",
					tick: 42,
					gate: {
						surprise: 1.2,
						z: 3,
						warm: false,
						cold: "history",
						passed: false,
					},
					blocked: true,
					events: [
						{
							kind: "reuse",
							rule: 1,
							into: 0,
							from: 0,
							digram: ["R02", "R04"],
						},
					],
					policy: {
						epoch: "1",
						paths: 3,
						trading: true,
						stored: false,
						match: "",
						depth: 0,
						action: "",
						outcome: "",
					},
					rules: [
						{
							id: 1,
							name: "M1",
							count: 2,
							depth: 2,
							length: 2,
							symbols: ["R02", "R04"],
							terminals: ["R02", "R04"],
						},
					],
					violations: ["digram uniqueness violated"],
				}),
			),
		]);

		expect(sample.feeds).toBe(2);
		expect(sample.rules).toBe(2);
		expect(sample.grammarSymbols).toBe(10);
		expect(sample.rawLength).toBe(10);
		expect(sample.avgRuleDepth).toBeCloseTo(1.5);
		expect(sample.maxRuleDepth).toBe(2);
		expect(sample.actionCoverage).toBeCloseTo(0.5);
		// 3 events on the first frame (1 create), 1 on the second (1 reuse).
		expect(sample.createRate).toBeCloseTo(1 / 4);
		expect(sample.reuseRate).toBeCloseTo(1 / 4);
		expect(sample.tokenEmitRate).toBeCloseTo(0.5);
		expect(sample.blockedRate).toBeCloseTo(0.5);
		expect(sample.policyStoredRate).toBeCloseTo(0.5);
		expect(sample.outcomeRate).toBeCloseTo(0.5);
		expect(sample.violations).toBe(1);
	});

	it("keeps a missing path book blank and a reported zero as zero", () => {
		ingestLongtailTick({ gatePass: 1, gateTotal: 4, litTicks: 4 });
		const missing = summarizeTraining([]);
		expect(missing.tokenEmitRate).toBeCloseTo(0.25);
		expect(missing.developedPaths).toBeNull();
		expect(missing.exitProfit).toBeNull();
		expect(missing.vetoRate).toBeNull();

		resetLongtailRecorder();

		ingestLongtailTick({
			gatePass: 0,
			gateTotal: 4,
			developedPaths: 0,
			promotedPaths: 0,
			prunedPaths: 1,
			exitProfit: 0,
			exitStop: 2,
			exitBranch: 0,
		});
		const reported = summarizeTraining([]);
		expect(reported.developedPaths).toBe(0);
		expect(reported.prunedPaths).toBe(1);
		expect(reported.meanThreshold).toBeNull();
		expect(reported.exitStop).toBe(2);
		expect(reported.tokenEmitRate).toBe(0);
	});
});

describe("tone thresholds", () => {
	it("paints a bad number red and a good number green, never the reverse", () => {
		expect(winRateTone(null)).toBe("neutral");
		expect(winRateTone(0.6)).toBe("good");
		expect(winRateTone(0.5)).toBe("warn");
		expect(winRateTone(0.3)).toBe("bad");

		expect(returnTone(null)).toBe("neutral");
		expect(returnTone(0.01)).toBe("good");
		expect(returnTone(-0.005)).toBe("warn");
		expect(returnTone(-0.1)).toBe("bad");

		expect(surpriseTone(0.4)).toBe("good");
		expect(surpriseTone(1)).toBe("warn");
		expect(surpriseTone(3)).toBe("bad");
		expect(surpriseTone(-0.1)).toBe("bad");

		expect(reuseTone(0.6)).toBe("good");
		expect(reuseTone(0.3)).toBe("warn");
		expect(reuseTone(0.1)).toBe("bad");

		expect(coverageTone(0.6)).toBe("good");
		expect(coverageTone(0.1)).toBe("bad");

		expect(matchTone(0.6)).toBe("good");
		expect(matchTone(0.1)).toBe("bad");

		expect(tokenEmitTone(0.2)).toBe("good");
		expect(tokenEmitTone(0.08)).toBe("warn");
		expect(tokenEmitTone(0.02)).toBe("bad");
		expect(blockedTone(0.5)).toBe("good");
		expect(blockedTone(0.95)).toBe("bad");

		expect(warmTone(0.8)).toBe("good");
		expect(warmTone(0.3)).toBe("bad");
	});

	it("maps tones to the theme's contextual colours", () => {
		expect(toneColor("good")).toBe("var(--up)");
		expect(toneColor("warn")).toBe("var(--warn)");
		expect(toneColor("bad")).toBe("var(--down)");
		expect(toneColor("info")).toBe("var(--info)");
		expect(toneColor("neutral")).toBe("var(--f3)");
	});

	it("maps tones to the UI library's semantic variants", () => {
		expect(toneVariant("good")).toBe("success");
		expect(toneVariant("warn")).toBe("warning");
		expect(toneVariant("bad")).toBe("error");
		expect(toneVariant("info")).toBe("info");
		expect(toneVariant("neutral")).toBe("disabled");
		expect(alertVariant("neutral")).toBe("info");
	});
});

const emptyStops: OpenLotTelemetry = {
	open: null,
	invalidation: null,
	evaporation: null,
	trailing: null,
	expiry: null,
	armed: null,
	maxProximity: null,
	exits: {},
	exitTotal: 0,
};

const sample = (
	overrides: Partial<{
		paper: Partial<BookDeskStats>;
		virtual: Partial<BookDeskStats>;
		training: Partial<TrainingSample>;
		stops: Partial<OpenLotTelemetry>;
	}> = {},
): LongtailSample => ({
	at: Date.now(),
	paper: {
		closed: 0,
		open: 0,
		wins: 0,
		winRate: null,
		meanReturn: null,
		medianReturn: null,
		worstReturn: null,
		avgHoldNs: null,
		...overrides.paper,
	},
	virtual: {
		closed: 0,
		open: 0,
		wins: 0,
		winRate: null,
		meanReturn: null,
		medianReturn: null,
		worstReturn: null,
		avgHoldNs: null,
		...overrides.virtual,
	},
	training: {
		feeds: 0,
		rules: null,
		grammarSymbols: null,
		rawLength: null,
		avgRuleDepth: null,
		maxRuleDepth: null,
		actionCoverage: null,
		createRate: null,
		reuseRate: null,
		inlineRate: null,
		tokenEmitRate: null,
		blockedRate: null,
		blockedBrightnessRate: null,
		blockedDuplicateRate: null,
		vetoRate: null,
		emptyRate: null,
		policyStoredRate: null,
		outcomeRate: null,
		outcomeEnterRate: null,
		outcomeExitRate: null,
		violations: null,
		exploredEnter: null,
		venueOpen: null,
		virtualExcursion: null,
		developedPaths: null,
		promotedPaths: null,
		prunedPaths: null,
		meanThreshold: null,
		exitProfit: null,
		exitStop: null,
		exitBranch: null,
		...overrides.training,
	},
	stops: { ...emptyStops, ...overrides.stops },
});

describe("verdict", () => {
	it("reports warming up without data", () => {
		expect(verdict([], "virtual")).toEqual({
			label: "Warming up",
			tone: "neutral",
		});
		expect(verdict([sample()], "virtual")).toEqual({
			label: "Warming up",
			tone: "neutral",
		});
	});

	it("reports improving from good levels and an improving edge", () => {
		const samples = [
			sample({
				virtual: { closed: 4, wins: 3, winRate: 0.6, meanReturn: 0.01 },
				training: {
					rules: 10,
					reuseRate: 0.6,
					policyStoredRate: 0.05,
					tokenEmitRate: 0.08,
					blockedRate: 0.7,
				},
			}),
			sample({
				virtual: { closed: 5, wins: 4, winRate: 0.65, meanReturn: 0.03 },
				training: {
					rules: 11,
					reuseRate: 0.6,
					policyStoredRate: 0.05,
					tokenEmitRate: 0.08,
					blockedRate: 0.7,
				},
			}),
		];

		expect(verdict(samples, "virtual")).toEqual({
			label: "Improving",
			tone: "good",
		});
	});

	it("never reports improving from bad levels", () => {
		const samples = [
			sample({
				virtual: { closed: 5, wins: 1, winRate: 0.2, meanReturn: -0.1 },
				training: {
					rules: 3,
					reuseRate: 0.1,
					policyStoredRate: 0.01,
					tokenEmitRate: 0.02,
					blockedRate: 0.9,
				},
			}),
			sample({
				virtual: { closed: 6, wins: 1, winRate: 0.15, meanReturn: -0.12 },
				training: {
					rules: 3,
					reuseRate: 0.1,
					policyStoredRate: 0.01,
					tokenEmitRate: 0.02,
					blockedRate: 0.9,
				},
			}),
		];

		expect(verdict(samples, "virtual")).toEqual({
			label: "Degrading",
			tone: "bad",
		});
	});

	it("calls a flat bad book stuck", () => {
		const held = sample({
			virtual: { closed: 5, wins: 1, winRate: 0.2, meanReturn: -0.1 },
			training: {
				rules: 3,
				reuseRate: 0.1,
				policyStoredRate: 0.01,
				tokenEmitRate: 0.02,
				blockedRate: 0.9,
			},
		});

		expect(verdict([held, held], "virtual")).toEqual({
			label: "Stuck",
			tone: "bad",
		});
	});

	it("calls a rising but still losing book recovering", () => {
		const samples = [
			sample({
				virtual: { closed: 5, wins: 1, winRate: 0.2, meanReturn: -0.1 },
				training: {
					rules: 3,
					reuseRate: 0.1,
					policyStoredRate: 0.01,
					tokenEmitRate: 0.02,
					blockedRate: 0.9,
				},
			}),
			sample({
				virtual: { closed: 6, wins: 2, winRate: 0.3, meanReturn: -0.04 },
				training: {
					rules: 3,
					reuseRate: 0.1,
					policyStoredRate: 0.01,
					tokenEmitRate: 0.02,
					blockedRate: 0.9,
				},
			}),
		];

		expect(verdict(samples, "virtual")).toEqual({
			label: "Recovering",
			tone: "warn",
		});
	});

	it("reports recovering when the book and the gate rise together", () => {
		const samples = [
			sample({
				virtual: { closed: 5, wins: 1, winRate: 0.2, meanReturn: -0.1 },
				training: {
					rules: 3,
					reuseRate: 0.1,
					policyStoredRate: 0.01,
					tokenEmitRate: 0.02,
					blockedRate: 0.9,
				},
			}),
			sample({
				virtual: { closed: 6, wins: 2, winRate: 0.3, meanReturn: -0.04 },
				training: {
					rules: 3,
					reuseRate: 0.1,
					policyStoredRate: 0.01,
					tokenEmitRate: 0.05,
					blockedRate: 0.85,
				},
			}),
		];

		expect(verdict(samples, "virtual")).toEqual({
			label: "Recovering",
			tone: "warn",
		});
	});

	it("reports slipping when good levels trend down", () => {
		const samples = [
			sample({
				virtual: { closed: 5, wins: 4, winRate: 0.7, meanReturn: 0.05 },
				training: {
					rules: 10,
					reuseRate: 0.6,
					policyStoredRate: 0.05,
					tokenEmitRate: 0.08,
					blockedRate: 0.7,
				},
			}),
			sample({
				virtual: { closed: 6, wins: 4, winRate: 0.6, meanReturn: 0.01 },
				training: {
					rules: 10,
					reuseRate: 0.6,
					policyStoredRate: 0.05,
					tokenEmitRate: 0.08,
					blockedRate: 0.7,
				},
			}),
		];

		expect(verdict(samples, "virtual")).toEqual({
			label: "Slipping",
			tone: "warn",
		});
	});
});

describe("trainingVerdict", () => {
	it("labels strong reuse and policy hits", () => {
		const samples = [
			sample({
				training: {
					reuseRate: 0.55,
					createRate: 0.2,
					policyStoredRate: 0.3,
					developedPaths: 4,
				},
			}),
			sample({
				training: {
					reuseRate: 0.6,
					createRate: 0.15,
					policyStoredRate: 0.35,
					developedPaths: 6,
				},
			}),
		];
		expect(trainingVerdict(samples)).toEqual({
			label: "Reusing paths",
			tone: "good",
		});
	});
});

describe("buildInsights", () => {
	it("reports the warmup state without data", () => {
		const insights = buildInsights([], "virtual");

		expect(insights).toHaveLength(1);
		expect(insights[0].id).toBe("warmup");
		expect(insights[0].tone).toBe("neutral");
	});

	it("writes contextual readings for a populated sample", () => {
		const insights = buildInsights(
			[
				sample({
					virtual: { closed: 5, wins: 1, winRate: 0.2, meanReturn: -0.1 },
					training: {
						rules: 3,
						avgRuleDepth: 1.5,
						maxRuleDepth: 2,
						reuseRate: 0.1,
						createRate: 0.8,
						tokenEmitRate: 0.03,
						blockedRate: 0.85,
						policyStoredRate: 0.01,
						outcomeRate: 0.1,
					},
				}),
				sample({
					virtual: { closed: 6, wins: 1, winRate: 0.15, meanReturn: -0.12 },
					training: {
						rules: 3,
						avgRuleDepth: 1.5,
						maxRuleDepth: 2,
						reuseRate: 0.1,
						createRate: 0.8,
						tokenEmitRate: 0.03,
						blockedRate: 0.85,
						policyStoredRate: 0.01,
						outcomeRate: 0.1,
					},
				}),
			],
			"virtual",
		);

		const byId = Object.fromEntries(insights.map((i) => [i.id, i]));
		expect(byId.grammar.tone).toBe("bad");
		expect(byId.grammar.body).toContain("reuse");
		expect(byId.policy.body).toContain("stored path");
		expect(byId.paths.tone).toBe("neutral");
		expect(byId.emit.body).toContain("new letter");
		expect(byId.exits.tone).toBe("neutral");
	});

	it("reads developed paths and exit reasons", () => {
		const insights = buildInsights(
			[
				sample({
					virtual: { closed: 5, wins: 4, winRate: 0.8, meanReturn: 0.05 },
					training: {
						developedPaths: 12,
						promotedPaths: 2,
						prunedPaths: 1,
						meanThreshold: 0.4,
						exitProfit: 4,
						exitStop: 1,
						exitBranch: 2,
						tokenEmitRate: 0.08,
					},
				}),
			],
			"virtual",
		);

		const paths = insights.find((i) => i.id === "paths");
		const exits = insights.find((i) => i.id === "exits");
		expect(paths?.body).toContain("12 developed");
		expect(paths?.body).toContain("0.40");
		expect(exits?.body).toContain("4 cleared the spread");
		expect(exits?.tone).toBe("good");
	});
});

const epochSample = (at: number, tokenEmitRate: number | null = 0.5) => ({
	at,
	paper: {
		closed: 1,
		open: 0,
		wins: 1,
		winRate: 1,
		meanReturn: 0.1,
		medianReturn: 0.1,
		worstReturn: 0.1,
		avgHoldNs: 1_000,
	},
	virtual: {
		closed: 0,
		open: 0,
		wins: 0,
		winRate: null,
		meanReturn: null,
		medianReturn: null,
		worstReturn: null,
		avgHoldNs: null,
	},
	training: {
		feeds: 1,
		rules: null,
		tokenEmitRate,
		exploredEnter: 18,
	},
	stops: emptyStops,
});

describe("epoch history", () => {
	it("ignores a batch that arrives before the replay begins", () => {
		expect(ingestLongtailHistory([epochSample(1_000)])).toBe(false);
		expect(longtailStore.state.samples).toHaveLength(0);
	});

	it("replaces samples collected after refresh with the epoch", () => {
		longtailStore.setState(() => ({
			samples: [sample({ paper: { closed: 0 } })],
			lastSampleAt: 1,
		}));

		beginLongtailHistory();
		expect(
			ingestLongtailHistory([epochSample(5_000), { at: "late" }, null]),
		).toBe(true);
		expect(ingestLongtailHistory([epochSample(10_000, 0)])).toBe(true);
		endLongtailHistory();

		const ats = longtailStore.state.samples.map((row) => row.at);
		expect(ats.slice(0, 2)).toEqual([5_000, 10_000]);
		expect(longtailStore.state.samples[0]?.training.tokenEmitRate).toBe(0.5);
		expect(longtailStore.state.samples[0]?.paper.meanReturn).toBe(0.1);
		expect(longtailStore.state.samples[0]?.training.rules).toBeNull();
		expect(longtailStore.state.samples[1]?.training.tokenEmitRate).toBe(0);
	});

	it("keeps the newest chart window of a long epoch", () => {
		beginLongtailHistory();
		const batch = Array.from({ length: 800 }, (_, i) =>
			epochSample(1_000 + i * 5_000),
		);
		ingestLongtailHistory(batch);
		endLongtailHistory();

		const rows = longtailStore.state.samples;
		const newest = 1_000 + 799 * 5_000;

		expect(rows).toHaveLength(CHART_SAMPLES);
		expect(rows.some((row) => row.at === 1_000)).toBe(false);
		expect(rows.some((row) => row.at === newest)).toBe(true);
	});

	it("leaves the page alone when the epoch tape is empty", () => {
		const kept = sample();
		longtailStore.setState(() => ({ samples: [kept], lastSampleAt: kept.at }));

		beginLongtailHistory();
		endLongtailHistory();

		expect(longtailStore.state.samples).toEqual([kept]);
	});
});

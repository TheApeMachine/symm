import { afterEach, describe, expect, it } from "vitest";
import {
	alertVariant,
	type BookDeskStats,
	buildInsights,
	coverageTone,
	gatePassTone,
	type LongtailSample,
	longtailStore,
	matchTone,
	returnTone,
	reuseTone,
	summarizeBook,
	summarizeTraining,
	surpriseTone,
	type TrainingSample,
	toneColor,
	toneVariant,
	verdict,
	warmTone,
	winRateTone,
} from "./longtail";
import type { SequiturFeed } from "./sequitur";
import { type SequiturFrame, sequiturFrame } from "./sequitur-fixture";

afterEach(() => {
	longtailStore.setState(() => ({ samples: [], lastSampleAt: 0 }));
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
) => ({
	id: `${symbol}-${virtual}-${netReturnPct}`,
	symbol,
	virtual,
	closed_at: "2026-01-01T00:00:00Z",
	cost: 100,
	proceeds: 100 * (1 + netReturnPct / 100),
	net_return: netReturnPct,
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
		expect(sample.gatePassRate).toBeNull();
		expect(sample.reuseRate).toBeNull();
		expect(sample.policyMatchRate).toBeNull();
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
		expect(sample.gatePassRate).toBeCloseTo(0.5);
		expect(sample.blockedRate).toBeCloseTo(0.5);
		expect(sample.warmRate).toBeCloseTo(0.5);
		expect(sample.avgSurprise).toBeCloseTo((0.42 + 1.2) / 2);
		expect(sample.avgZ).toBeCloseTo((2.1 + 3) / 2);
		expect(sample.policyStoredRate).toBeCloseTo(0.5);
		expect(sample.policyMatchRate).toBeCloseTo(0.5);
		expect(sample.avgMatchDepth).toBe(2);
		expect(sample.outcomeRate).toBeCloseTo(0.5);
		expect(sample.violations).toBe(1);
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

		expect(reuseTone(0.6)).toBe("good");
		expect(reuseTone(0.3)).toBe("warn");
		expect(reuseTone(0.1)).toBe("bad");

		expect(coverageTone(0.6)).toBe("good");
		expect(coverageTone(0.1)).toBe("bad");

		expect(matchTone(0.6)).toBe("good");
		expect(matchTone(0.1)).toBe("bad");

		expect(gatePassTone(0.8)).toBe("good");
		expect(gatePassTone(0.3)).toBe("bad");

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

const sample = (
	overrides: Partial<{
		paper: Partial<BookDeskStats>;
		virtual: Partial<BookDeskStats>;
		training: Partial<TrainingSample>;
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
		gatePassRate: null,
		blockedRate: null,
		warmRate: null,
		avgSurprise: null,
		avgZ: null,
		policyStoredRate: null,
		policyMatchRate: null,
		avgMatchDepth: null,
		outcomeRate: null,
		violations: null,
		pathStableRate: null,
		frictionPassRate: null,
		exploredEnter: null,
		confirmedEnter: null,
		venueOpen: null,
		virtualExcursion: null,
		meanMaturity: null,
		...overrides.training,
	},
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
					policyMatchRate: 0.6,
					gatePassRate: 0.8,
				},
			}),
			sample({
				virtual: { closed: 5, wins: 4, winRate: 0.65, meanReturn: 0.03 },
				training: {
					rules: 11,
					reuseRate: 0.6,
					policyMatchRate: 0.6,
					gatePassRate: 0.8,
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
					policyMatchRate: 0.1,
					gatePassRate: 0.2,
				},
			}),
			sample({
				virtual: { closed: 6, wins: 1, winRate: 0.15, meanReturn: -0.12 },
				training: {
					rules: 3,
					reuseRate: 0.1,
					policyMatchRate: 0.1,
					gatePassRate: 0.2,
				},
			}),
		];

		expect(verdict(samples, "virtual")).toEqual({
			label: "Degrading",
			tone: "bad",
		});
	});

	it("reports slipping when good levels trend down", () => {
		const samples = [
			sample({
				virtual: { closed: 5, wins: 4, winRate: 0.7, meanReturn: 0.05 },
				training: {
					rules: 10,
					reuseRate: 0.6,
					policyMatchRate: 0.6,
					gatePassRate: 0.8,
				},
			}),
			sample({
				virtual: { closed: 6, wins: 4, winRate: 0.6, meanReturn: 0.01 },
				training: {
					rules: 10,
					reuseRate: 0.6,
					policyMatchRate: 0.6,
					gatePassRate: 0.8,
				},
			}),
		];

		expect(verdict(samples, "virtual")).toEqual({
			label: "Slipping",
			tone: "warn",
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
						gatePassRate: 0.3,
						warmRate: 0.5,
						avgSurprise: 1.5,
						policyStoredRate: 0.5,
						policyMatchRate: 0.1,
						avgMatchDepth: 2,
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
						gatePassRate: 0.3,
						warmRate: 0.5,
						avgSurprise: 1.5,
						policyStoredRate: 0.5,
						policyMatchRate: 0.1,
						avgMatchDepth: 2,
						outcomeRate: 0.1,
					},
				}),
			],
			"virtual",
		);

		const byId = Object.fromEntries(insights.map((i) => [i.id, i]));
		expect(byId.edge.tone).toBe("bad");
		expect(byId.edge.body).toContain("-12.0%");
		expect(byId.edge.body).toContain("slipping");
		expect(byId.grammar.tone).toBe("bad");
		expect(byId.gate.tone).toBe("bad");
		expect(byId.policy.tone).toBe("bad");
	});

	it("reports invariant violations in red", () => {
		const insights = buildInsights(
			[
				sample({
					virtual: { closed: 5, wins: 4, winRate: 0.8, meanReturn: 0.05 },
					training: {
						rules: 10,
						reuseRate: 0.6,
						gatePassRate: 0.8,
						policyStoredRate: 0.5,
						policyMatchRate: 0.6,
						violations: 2,
					},
				}),
			],
			"virtual",
		);

		const violations = insights.find((i) => i.id === "violations");
		expect(violations?.tone).toBe("bad");
	});
});

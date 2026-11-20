import { describe, expect, it } from "vitest";
import {
	classifyExit,
	emptyBookStats,
	emptyExitMix,
	emptyPaths,
	emptyPolicy,
	emptyStops,
	type LongtailSample,
	summarizeExits,
	summarizePathBook,
	summarizePolicy,
	verdict,
} from "#/collections/longtail";

const baseSample = (
	at: number,
	overrides: Partial<LongtailSample> = {},
): LongtailSample => ({
	at,
	paper: emptyBookStats(),
	virtual: emptyBookStats(),
	real: emptyBookStats(),
	paperWin: null,
	paperEdge: null,
	realWin: null,
	realEdge: null,
	realArmed: null,
	eligibleResistance: 0.2,
	paths: emptyPaths(),
	policy: emptyPolicy(),
	exits: {
		virtual: emptyExitMix(),
		paper: emptyExitMix(),
		real: emptyExitMix(),
	},
	stops: {
		virtual: emptyStops(),
		paper: emptyStops(),
		real: emptyStops(),
	},
	...overrides,
});

describe("classifyExit", () => {
	it("buckets profit, stop, and policy triggers", () => {
		expect(classifyExit("cleared friction")).toBe("profit");
		expect(classifyExit("trailing take-profit")).toBe("profit");
		expect(classifyExit("invalidation stop")).toBe("stop");
		expect(classifyExit("structural exhaustion: failed retest")).toBe("stop");
		expect(classifyExit("exit branch")).toBe("policy");
		expect(classifyExit("manual")).toBe("other");
	});
});

describe("summarizePathBook", () => {
	it("counts developed and promoted leaves and reports churn", () => {
		const snapshot = summarizePathBook(
			{
				"model/paths/A/B/enter.bin": 0.15,
				"model/paths/A/C/enter.bin": 0.5,
				"model/paths/D/E/enter.bin": 0.9,
			},
			[
				"model/paths/A/B/enter.bin",
				"model/paths/A/C/enter.bin",
				"model/paths/D/E/enter.bin",
			],
			0.2,
			new Set(["model/paths/A/C/enter.bin", "model/paths/gone/enter.bin"]),
		);

		expect(snapshot.developed).toBe(2);
		expect(snapshot.promoted).toBe(1);
		expect(snapshot.meanResistance).toBeCloseTo(0.325);
		expect(snapshot.added).toBe(2);
		expect(snapshot.removed).toBe(1);
	});
});

describe("summarizePolicy", () => {
	it("returns hit rate over live matches", () => {
		expect(
			summarizePolicy({
				"BTC/USD": { stored: true },
				"ETH/USD": { stored: false },
				"SOL/USD": { stored: true },
			}),
		).toEqual({ checks: 3, hits: 2, hitRate: 2 / 3 });
	});
});

describe("summarizeExits", () => {
	it("counts triggers for one book only", () => {
		const mix = summarizeExits(
			[
				{
					id: "a",
					symbol: "A/USD",
					entry_path: "",
					exit_path: "",
					opened_at: "2026-01-01T00:00:00Z",
					tier_key: "paper",
					closed_at: "2026-01-01T00:00:00Z",
					pnl_pct: 1,
					outcome_triggers: ["cleared friction"],
				},
				{
					id: "b",
					symbol: "B/USD",
					entry_path: "",
					exit_path: "",
					opened_at: "2026-01-01T00:00:00Z",
					tier_key: "virtual",
					closed_at: "2026-01-01T00:00:00Z",
					pnl_pct: -1,
					outcome_triggers: ["invalidation stop"],
				},
				{
					id: "c",
					symbol: "C/USD",
					entry_path: "",
					exit_path: "",
					opened_at: "2026-01-01T00:00:00Z",
					tier_key: "paper",
					closed_at: "2026-01-01T00:01:00Z",
					pnl_pct: -0.5,
					exit_trigger: "exit branch",
				},
			],
			"paper",
		);

		expect(mix).toEqual({
			profit: 1,
			stop: 0,
			policy: 1,
			other: 0,
			total: 2,
		});
	});
});

describe("verdict", () => {
	it("labels improving when selected-tier edge and win rate rise while positive", () => {
		const samples = [
			baseSample(1, {
				paper: {
					...emptyBookStats(),
					closed: 4,
					wins: 3,
					winRate: 0.6,
					meanReturn: 0.01,
				},
				paperWin: 0.6,
				paperEdge: 0.01,
			}),
			baseSample(2, {
				paper: {
					...emptyBookStats(),
					closed: 8,
					wins: 6,
					winRate: 0.7,
					meanReturn: 0.03,
				},
				paperWin: 0.7,
				paperEdge: 0.03,
			}),
		];

		expect(verdict(samples, "paper")).toEqual({
			label: "Improving",
			tone: "good",
		});
	});

	it("labels thrashing when edge flips around zero", () => {
		const samples = Array.from({ length: 8 }, (_, i) =>
			baseSample(i + 1, {
				paper: {
					...emptyBookStats(),
					closed: 4,
					wins: 2,
					winRate: 0.5,
					meanReturn: i % 2 === 0 ? 0.02 : -0.02,
				},
				paperWin: 0.5,
				paperEdge: i % 2 === 0 ? 0.02 : -0.02,
			}),
		);

		expect(verdict(samples, "paper").label).toBe("Thrashing");
	});
});

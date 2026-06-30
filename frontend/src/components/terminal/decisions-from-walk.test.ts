import { describe, expect, it } from "vitest";
import type { WalkTrace } from "#/collections/playbook";
import {
	decisionRowFromWalk,
	mergeTerminalDecisionRows,
	terminalDecisionsFromWalk,
	walkVerdict,
} from "#/components/terminal/decisions-from-walk";
import type { TerminalKernel } from "#/components/terminal/model";

const kernel = (source: string, confidencePercent: number): TerminalKernel => ({
	source,
	name: source,
	category: source,
	status: "measured",
	statusLabel: "measured",
	strengthText: "0.1000",
	confidencePercent,
	surprisePercent: 40,
	healthPercent: 100,
	confidenceText: (confidencePercent / 100).toFixed(2),
	surpriseText: "1.00",
	samplesText: "12/24",
	activeText: "1/8",
	observedText: "live",
	faultText: "",
});

const walk = (
	symbol: string,
	outcome: WalkTrace["steps"][number]["outcome"],
): WalkTrace => ({
	symbol,
	steps: [{ path: [0], outcome, reason: "matched_branch" }],
});

describe("decisions from walk", () => {
	it("maps action walks to allow verdicts", () => {
		expect(walkVerdict([{ path: [0], outcome: "action" }])).toBe("allow");
	});

	it("builds dashboard rows from accumulated walk evaluations", () => {
		const rows = terminalDecisionsFromWalk(
			{
				"SOL/USD": walk("SOL/USD", "action"),
				"ETH/USD": walk("ETH/USD", "rejected"),
			},
			[kernel("pumpdump", 64), kernel("causal", 41)],
		);

		expect(rows.map((row) => row.symbol)).toEqual(["SOL/USD", "ETH/USD"]);
		expect(rows[0]?.verdict).toBe("allow");
		expect(rows[1]?.verdict).toBe("blocked");
		expect(rows[0]?.scoreText).toBe("0.525");
		expect(rows[0]?.edgeText).toContain("/");
	});

	it("scores each walk row from that symbol's kernels", () => {
		const rows = terminalDecisionsFromWalk(
			{
				"SOL/USD": walk("SOL/USD", "rejected"),
				"ETH/USD": walk("ETH/USD", "rejected"),
			},
			(symbol) =>
				symbol === "SOL/USD"
					? [kernel("resonance", 100)]
					: [kernel("resonance", 10)],
		);

		const sol = rows.find((row) => row.symbol === "SOL/USD");
		const eth = rows.find((row) => row.symbol === "ETH/USD");

		expect(sol?.scoreValue).toBeGreaterThan(eth?.scoreValue ?? 0);
		expect(eth?.scoreText).not.toBe("1.000");
	});

	it("treats a score exactly at the entry line as clearing the edge", () => {
		const row = decisionRowFromWalk(
			{
				symbol: "ADA/USD",
				active_path: [0, 1, 2],
				steps: [{ path: [0], outcome: "action", reason: "matched_branch" }],
			},
			[kernel("pumpdump", 100)],
			1,
		);

		expect(row.scoreValue).toBe(1);
		expect(row.edgeText).toBe("+0.000 / 1.000");
		expect(row.edgePositive).toBe(true);
	});

	it("merges walk and trace rows by symbol instead of replacing", () => {
		const merged = mergeTerminalDecisionRows(
			[
				{
					key: "BTC/USD:walk",
					symbol: "BTC/USD",
					source: "walk",
					scoreText: "0.400",
					scoreValue: 0.4,
					verdict: "in-play",
					why: "matched branch",
					signals: [],
					edgeText: "+0.000 / 0.400",
					edgePositive: false,
				},
			],
			[
				{
					key: "DASH/USD:decision",
					symbol: "DASH/USD",
					source: "toxicity",
					scoreText: "1.000",
					scoreValue: 1,
					verdict: "blocked",
					why: "below entry",
					signals: [{ source: "toxicity", confidence: 1 }],
					edgeText: "+0.000 / 1.000",
					edgePositive: false,
				},
			],
		);

		expect(merged.map((row) => row.symbol).sort()).toEqual([
			"BTC/USD",
			"DASH/USD",
		]);
	});

	it("keeps backend trader verdicts when a walk row exists for the symbol", () => {
		const merged = mergeTerminalDecisionRows(
			[
				{
					key: "BTC/USD:walk",
					symbol: "BTC/USD",
					source: "walk",
					scoreText: "0.850",
					scoreValue: 0.85,
					verdict: "allow",
					why: "action",
					signals: [{ source: "pumpdump", confidence: 0.85 }],
					edgeText: "+0.100 / 0.750",
					edgePositive: true,
				},
			],
			[
				{
					key: "BTC/USD:decision",
					symbol: "BTC/USD",
					source: "decision",
					scoreText: "0.000",
					scoreValue: 0,
					verdict: "blocked",
					why: "field risk",
					signals: [],
					edgeText: "-0.000 / 0.000",
					edgePositive: false,
				},
			],
		);

		expect(merged).toHaveLength(1);
		expect(merged[0]).toMatchObject({
			symbol: "BTC/USD",
			scoreValue: 0,
			verdict: "blocked",
			why: "field risk",
		});
	});
});

import { describe, expect, it } from "vitest";
import type { GateForces } from "./gate-labels";
import {
	formatGateBadge,
	formatTrieLine,
	resolveEnterExecutionBook,
} from "./gate-labels";

const baseGate: GateForces = {
	phase: "enter",
	passed: true,
	virtual: false,
	treeVirtual: false,
	treeImpulse: 1.2,
	treeResistance: 1.0,
	treeAction: "enter",
};

describe("resolveEnterExecutionBook", () => {
	it("prefers virtual when the trie has not cleared resistance", () => {
		expect(
			resolveEnterExecutionBook(
				{ ...baseGate, treeVirtual: true, passed: true },
				false,
			),
		).toBe("virtual");
	});

	it("returns paper when gate and trie both allow paper", () => {
		expect(resolveEnterExecutionBook(baseGate, false)).toBe("paper");
	});

	it("returns blocked when the enter gate vetoed the tick", () => {
		expect(
			resolveEnterExecutionBook(
				{ ...baseGate, passed: false, virtual: false },
				true,
			),
		).toBe("blocked");
	});
});

describe("formatGateBadge", () => {
	it("labels hold phase by the open lot book, not enter paper ok", () => {
		const badge = formatGateBadge({
			gateLive: true,
			phase: "hold",
			gate: { ...baseGate, phase: "hold", passed: true },
			blocked: false,
			positionBook: "paper",
		});
		expect(badge.label).toBe("Paper book · open");
		expect(badge.label).not.toContain("paper ok");
	});

	it("names virtual explore when gate or trie demotes enter", () => {
		const badge = formatGateBadge({
			gateLive: true,
			phase: "enter",
			gate: { ...baseGate, virtual: true, passed: false },
			blocked: false,
			positionBook: "paper",
		});
		expect(badge.label).toBe("Enter · virtual explore");
	});
});

describe("formatTrieLine", () => {
	it("explains enter promotion against path resistance", () => {
		const line = formatTrieLine(baseGate, "enter");
		expect(line).toContain("clears");
		expect(line).toContain("paper-eligible");
	});
});

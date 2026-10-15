// @vitest-environment jsdom

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
	buildNestedCapsules,
	generateSequiturTrace,
	grammarResponseToSnapshot,
} from "./sequitur-engine";
import { SequiturGrammarViz } from "./sequitur-grammar-viz";

describe("Sequitur Engine", () => {
	it("induces hierarchical rules from repeating digrams", () => {
		const trace = generateSequiturTrace("abcdbcabcd");
		expect(trace.length).toBeGreaterThan(5);

		const finalSnapshot = trace[trace.length - 1];
		expect(finalSnapshot.rules.length).toBeGreaterThan(0);
		expect(finalSnapshot.invariantStatus.digramUniqueness).not.toBe("VIOLATED");
		expect(finalSnapshot.invariantStatus.ruleUtility).not.toBe("VIOLATED");
	});

	it("converts backend SequiturGrammarResponse into GrammarSnapshot", () => {
		const mockResponse = {
			symbol: "BTC/USD",
			rules: [
				{
					id: 1,
					name: "M1",
					count: 3,
					depth: 1,
					length: 2,
					symbols: ["R02", "R04"],
					terminals: ["R02", "R04"],
					action: "enter",
				},
			],
			start: ["M1", "R05", "M1"],
		};

		const snapshot = grammarResponseToSnapshot(mockResponse, {
			surprise: 0.042,
			activeTokens: "R02/R04",
		});

		expect(snapshot.rules.length).toBe(1);
		expect(snapshot.rules[0].id).toBe("M1");
		expect(snapshot.rules[0].action).toBe("enter");
		expect(snapshot.metrics.surprise).toBe(0.042);
		expect(snapshot.metrics.activeTokens).toBe("R02/R04");
	});

	it("does not collide terminal regions R10..R14 with rules M10..M14", () => {
		const mockResponse = {
			symbol: "BTC/USD",
			rules: [
				{
					id: 12,
					name: "M12",
					count: 2,
					depth: 1,
					length: 2,
					symbols: ["R02", "R09"],
					terminals: ["R02", "R09"],
				},
				{
					id: 1,
					name: "M1",
					count: 2,
					depth: 1,
					length: 2,
					symbols: ["R12", "R01"],
					terminals: ["R12", "R01"],
				},
			],
			start: ["M1", "R12", "M12"],
		};

		const snapshot = grammarResponseToSnapshot(mockResponse);
		const m1 = snapshot.rules.find((r) => r.id === "M1");
		expect(m1).toBeDefined();
		// In M1 -> R12 R01, R12 must be terminal, not a rule!
		expect(m1?.symbols[0].isRule).toBe(false);
		expect(m1?.symbols[0].ruleId).toBeUndefined();
		expect(m1?.symbols[0].value).toBe("R12");

		// In start: M1, R12, M12
		expect(snapshot.rootRule.symbols[0].isRule).toBe(true);
		expect(snapshot.rootRule.symbols[0].value).toBe("M1");
		expect(snapshot.rootRule.symbols[1].isRule).toBe(false);
		expect(snapshot.rootRule.symbols[1].value).toBe("R12");
		expect(snapshot.rootRule.symbols[2].isRule).toBe(true);
		expect(snapshot.rootRule.symbols[2].value).toBe("M12");
	});

	it("builds nested capsule hierarchy for telescoping lens", () => {
		const trace = generateSequiturTrace("abracadabra");
		const snapshot = trace[trace.length - 1];
		const capsules = buildNestedCapsules(snapshot);

		expect(capsules.length).toBeGreaterThan(0);
		const ruleCapsule = capsules.find((c) => c.isRule);
		expect(ruleCapsule).toBeDefined();
		if (ruleCapsule) {
			expect(ruleCapsule.children).toBeDefined();
		}
	});
});

describe("SequiturGrammarViz UI", () => {
	it("renders main live toolbar, badges, and metrics", () => {
		render(<SequiturGrammarViz />);

		expect(screen.getByText(/Sequitur Grammar/i)).toBeTruthy();
		expect(screen.getByText(/LIVE/i)).toBeTruthy();
		expect(screen.getByText("Surprise")).toBeTruthy();
		expect(screen.getByText("Chord")).toBeTruthy();
		expect(screen.getByText("Rules")).toBeTruthy();
		expect(screen.getByText("Ratio")).toBeTruthy();
		expect(screen.getByText(/awaiting grammar feed/i)).toBeTruthy();
		expect(screen.getByText(/awaiting production rules/i)).toBeTruthy();
	});
});

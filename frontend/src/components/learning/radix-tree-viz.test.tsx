// @vitest-environment jsdom
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
	buildTreeFromPrefixes,
	keyMatchesPrefix,
	keyMatchesSymbol,
	RadixTreeViz,
} from "./radix-tree-viz";
import type { SymbolProgress, TrieNodeData } from "./types";

const actionsOf = (node: TrieNodeData | null): string[] => {
	if (!node) return [];
	const found: string[] = [];
	const walk = (current: TrieNodeData) => {
		if (current.action) found.push(current.action);
		for (const child of current.children ?? []) walk(child);
	};
	walk(node);
	return found;
};

describe("buildTreeFromPrefixes", () => {
	it("keeps enter and exit when they share a grammar path", () => {
		const actions = actionsOf(
			buildTreeFromPrefixes([
				"policy/current/R01/R02/enter.bin",
				"policy/current/R01/R02/exit.bin",
			]),
		);
		expect(actions).toEqual(["ENTER", "EXIT"]);
	});
});

describe("keyMatchesPrefix", () => {
	it("ignores the policy/current store prefix", () => {
		expect(
			keyMatchesPrefix("policy/current/R01/R02/enter.bin", ["R01", "R02"]),
		).toBe(true);
	});

	it("returns true when key starts with the exact token sequence", () => {
		const key = "R01-R02/R03-R04/R05-R06/enter.json";
		expect(keyMatchesPrefix(key, ["R01-R02"])).toBe(true);
		expect(keyMatchesPrefix(key, ["R01-R02", "R03-R04"])).toBe(true);
		expect(keyMatchesPrefix(key, ["R01-R02", "R03-R04", "R05-R06"])).toBe(true);
	});

	it("returns false when a token differs in the prefix", () => {
		const key = "R01-R02/R03-R04/R05-R06/enter.json";
		expect(keyMatchesPrefix(key, ["R01-R03"])).toBe(false);
		expect(keyMatchesPrefix(key, ["R01-R02", "R05-R06"])).toBe(false);
	});

	it("returns false when key has fewer tokens than the prefix", () => {
		const key = "R01-R02/enter.json";
		expect(keyMatchesPrefix(key, ["R01-R02", "R03-R04"])).toBe(false);
	});
});

describe("keyMatchesSymbol", () => {
	it("matches via symbol.tokens prefix", () => {
		const symbol: SymbolProgress = {
			symbol: "BTC/USD",
			path: "R01/R02",
			depth: 2,
			tokens: ["R01", "R02"],
		};
		expect(keyMatchesSymbol("R01/R02/R03/R04/enter.json", symbol)).toBe(true);
		expect(keyMatchesSymbol("R01/R05/R03/R04/exit.json", symbol)).toBe(false);
	});

	it("matches via symbol.matched keys if present", () => {
		const symbol: SymbolProgress = {
			symbol: "BTC/USD",
			path: "R01/R02",
			depth: 2,
			tokens: ["R01", "R02"],
			matched: ["R01/R02/R03/R04/enter.json"],
		};
		expect(keyMatchesSymbol("R01/R02/R03/R04/enter.json", symbol)).toBe(true);
	});
});

describe("RadixTreeViz", () => {
	const sampleKeys = [
		"R01/R02/R03/enter.json",
		"R01/R02/R04/exit.json",
		"R05/R06/R07/enter.json",
		"R08/R09/R10/exit.json",
	];

	it("renders only matching branches for symbols with built up sequences", () => {
		const symbols: Record<string, SymbolProgress> = {
			"BTC/USD": {
				symbol: "BTC/USD",
				path: "R01/R02",
				depth: 2,
				tokens: ["R01", "R02"],
			},
		};

		const { container } = render(
			<RadixTreeViz keys={sampleKeys} symbols={symbols} />,
		);

		// Should show 2 matching branches (R01/R02/R03 and R01/R02/R04), not R05 or R08
		expect(container.textContent).toContain("2 matching branches");
		expect(container.textContent).toContain("1 active symbol");

		// Only nodes from the matching branch should exist (ROOT, R01, R02, R03, R04)
		expect(container.textContent).toContain("R01");
		expect(container.textContent).toContain("R02");
		expect(container.textContent).not.toContain("R05");
		expect(container.textContent).not.toContain("R08");

		const svg = container.querySelector("svg");
		expect(svg?.textContent).not.toContain("BTC");
	});

	it("shows informative empty state when no active symbol sequence exists", () => {
		const { container } = render(
			<RadixTreeViz keys={sampleKeys} symbols={{}} />,
		);
		expect(container.textContent).toContain("No Active Symbol Sequences");

		// User can toggle to view all branches
		const showAllBtn = screen.getByRole("button", {
			name: /Show all 4 stored branches/i,
		});
		fireEvent.click(showAllBtn);

		expect(container.textContent).toContain("Showing all 4 branches");
		expect(container.textContent).toContain("R05");
		expect(container.textContent).toContain("R08");
	});
});

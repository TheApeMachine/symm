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
	it("lays an explored path on the same terminals as a confirmed one", () => {
		const tree = buildTreeFromPrefixes([
			"model/explore/[R00-R03-R09-R00]/[R03-R05-R09]/[R01-R02-R03]/enter.bin",
		]);
		expect(tree?.children?.[0]?.token).toBe("[R00-R03-R09-R00]");
		expect(actionsOf(tree)).toEqual(["ENTER"]);
	});

	it("places enter and exit on separate branches from root", () => {
		const tree = buildTreeFromPrefixes([
			"policy/current/R01/R02/enter.bin",
			"policy/current/R04/R05/exit.bin",
		]);
		const enterLeaf = tree?.children?.find((c) => c.token === "R01")
			?.children?.[0]?.children?.find((c) => c.action === "ENTER");
		const exitLeaf = tree?.children?.find((c) => c.token === "R04")
			?.children?.[0]?.children?.find((c) => c.action === "EXIT");
		expect(enterLeaf?.action).toBe("ENTER");
		expect(exitLeaf?.action).toBe("EXIT");
		expect(enterLeaf?.prefix).not.toBe(exitLeaf?.prefix);
	});

	it("places explore enter and exit on different path terminals", () => {
		const tree = buildTreeFromPrefixes([
			"model/explore/R01/R02/enter.bin",
			"model/explore/R01/R02/R03/exit.bin",
		]);
		expect(actionsOf(tree)).toEqual(["ENTER", "EXIT"]);
		const enterLeaf = tree?.children
			?.find((c) => c.token === "R01")
			?.children?.find((c) => c.token === "R02")
			?.children?.find((c) => c.action === "ENTER");
		const exitLeaf = tree?.children
			?.find((c) => c.token === "R01")
			?.children?.find((c) => c.token === "R02")
			?.children?.find((c) => c.token === "R03")
			?.children?.find((c) => c.action === "EXIT");
		expect(enterLeaf?.action).toBe("ENTER");
		expect(exitLeaf?.action).toBe("EXIT");
	});

	it("defaults an unscored explore leaf to the 0.50 desk baseline", () => {
		const tree = buildTreeFromPrefixes(["model/explore/R01/R02/enter.bin"]);
		const leaf = tree?.children
			?.find((child) => child.token === "R01")
			?.children?.find((child) => child.token === "R02")
			?.children?.find((child) => child.action === "ENTER");
		expect(leaf?.threshold).toBe(0.5);
	});

	it("paints a leaf with its threshold and shares that with the branch", () => {
		const tree = buildTreeFromPrefixes(["model/explore/R01/R02/exit.bin"], {
			"model/explore/R01/R02/exit.bin": 0.8,
		});
		const leaf = tree?.children
			?.find((child) => child.token === "R01")
			?.children?.find((child) => child.token === "R02")
			?.children?.find((child) => child.action === "EXIT");

		expect(leaf?.threshold).toBe(0.8);
		expect(tree?.children?.[0]?.threshold).toBe(0.8);
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

	it("keeps every stored branch and still counts the live match", () => {
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

		fireEvent.click(screen.getByRole("button", { name: /All \(4\)/ }));
		expect(container.textContent).toContain("Showing all 4 branches");
		expect(container.textContent).toContain("R01");
		expect(container.textContent).toContain("R05");
		expect(container.textContent).toContain("R08");

		fireEvent.click(screen.getByRole("button", { name: /Matching \(2\)/ }));

		expect(container.textContent).toContain("2 matching branches");
		expect(container.textContent).toContain("1 active symbol");
		expect(container.textContent).not.toContain("R05");
		expect(container.textContent).not.toContain("R08");
	});

	it("prints the threshold under a scored leaf and fades the branch", () => {
		const { container } = render(
			<RadixTreeViz
				keys={["model/explore/R01/R02/exit.bin"]}
				thresholds={{ "model/explore/R01/R02/exit.bin": 0.8 }}
				symbols={{}}
			/>,
		);

		expect(container.textContent).toContain("0.80");
		expect(container.textContent).toContain("solid 0.05");
		const faded = container.querySelector("g[style*='opacity: 0.4']");
		expect(faded).not.toBeNull();
	});

	it("draws the stored trie when no symbol is walking it", () => {
		const { container } = render(
			<RadixTreeViz keys={sampleKeys} symbols={{}} />,
		);

		expect(container.textContent).toContain("All (4)");
		expect(container.textContent).toContain("R05");
		expect(container.textContent).toContain("R08");
		expect(container.textContent).not.toContain("No Active Symbol Sequences");
	});
});

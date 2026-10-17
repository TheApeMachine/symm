// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cognitionStore, ingestCognitionFrame } from "#/collections/cognition";
import { CognitiveTree, progressOf } from "./cognitive-tree";

const tree = {
	epoch: "1792195368165650000",
	paths: 1,
	trading: true,
	gate: {
		tickEpoch: "1792189318815706000",
		enabled: true,
		surpriseZ: 1,
		warmup:
			"settled: error average at 0.15 no longer below its average at 0.01",
		learningRate: 0.01,
	},
	keys: ["R12/R10/noop.bin"],
	branches: [
		{
			hash: "R12/R10",
			depth: 2,
			visits: 1,
			meanEdge: -0.033,
			confidence: 1,
			policy: "NOOP",
		},
	],
};

afterEach(() => {
	cleanup();
	cognitionStore.setState(() => ({ tree: null, matches: {} }));
	vi.restoreAllMocks();
});

describe("ingestCognitionFrame", () => {
	it("keeps the tree and per-symbol matches; a new tree drops old matches", () => {
		expect(ingestCognitionFrame("tree", tree)).toBe(true);
		expect(
			ingestCognitionFrame("match", {
				symbol: "BTC/USD",
				tick: 9,
				tokens: ["R12", "R10"],
				stored: true,
				match: "R12/R10",
				depth: 2,
				action: "noop",
			}),
		).toBe(true);
		expect(cognitionStore.state.matches["BTC/USD"].depth).toBe(2);

		ingestCognitionFrame("tree", { ...tree, epoch: "1792195368165650001" });
		expect(cognitionStore.state.matches).toEqual({});
	});

	it("refuses a malformed frame without applying it", () => {
		const error = vi.spyOn(console, "error").mockImplementation(() => {});
		expect(ingestCognitionFrame("tree", { ...tree, epoch: 1 })).toBe(false);
		expect(ingestCognitionFrame("match", { symbol: "BTC/USD" })).toBe(false);
		expect(ingestCognitionFrame("other", tree)).toBe(false);
		expect(cognitionStore.state.tree).toBeNull();
		expect(error).toHaveBeenCalledTimes(3);
	});
});

describe("progressOf", () => {
	it("points a stored match at its stored key", () => {
		expect(
			progressOf({
				symbol: "ETH/USD",
				tick: 3,
				tokens: ["R01", "R12", "R10"],
				stored: true,
				match: "R12/R10",
				depth: 2,
				action: "noop",
			}),
		).toEqual({
			symbol: "ETH/USD",
			path: "R01/R12/R10",
			depth: 2,
			tokens: ["R01", "R12", "R10"],
			matched: ["R12/R10/noop.bin"],
		});
	});
});

describe("CognitiveTree", () => {
	it("waits for the hub frame, without any fetch", () => {
		const fetchMock = vi.fn();
		vi.stubGlobal("fetch", fetchMock);
		render(<CognitiveTree />);
		expect(screen.getByTestId("cognitive-tree-empty").textContent).toContain(
			"waiting for the hub",
		);
		expect(fetchMock).not.toHaveBeenCalled();
		vi.unstubAllGlobals();
	});

	it("says paper trading is inactive when the hub reports no trained paths", () => {
		ingestCognitionFrame("tree", {
			...tree,
			paths: 0,
			trading: false,
			gate: null,
			keys: [],
			branches: [],
		});
		render(<CognitiveTree />);
		expect(screen.getByTestId("cognitive-tree-empty").textContent).toContain(
			"paper trading inactive: no trained paths",
		);
	});

	it("shows the loaded epoch, path count and gate", () => {
		ingestCognitionFrame("tree", tree);
		render(<CognitiveTree />);
		const meta = screen.getByTestId("cognitive-tree-meta").textContent ?? "";
		expect(meta).toContain("policy epoch 1792195368165650000");
		expect(meta).toContain("1 paths");
		expect(meta).toContain(
			"gated z≥1 · warm-up settled: error average at 0.15 no longer below its average at 0.01",
		);
	});
});

// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	cognitionStore,
	ingestCognitionFrame,
	ingestPolicyLayers,
	ingestTrainRegions,
	ingestTrainReject,
} from "#/collections/cognition";
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
	problem: null,
};

const mismatch = {
	...tree,
	epoch: "0",
	paths: 0,
	trading: false,
	gate: null,
	keys: [],
	branches: [],
	problem: {
		level: "error",
		reason:
			"newest policy epoch 7 was trained under gate enabled=false; this run is configured for gate enabled=true",
		fix: "re-run detect and train on the tick epoch 3",
	},
};

afterEach(() => {
	cleanup();
	cognitionStore.setState(() => ({
		epoch: "0",
		policyKeys: [],
		matches: {},
		problem: null,
		tree: null,
	}));
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
		rejected: [],
	});
	});
});

describe("CognitiveTree", () => {
	it("waits for the hub frame, without any fetch", () => {
		const fetchMock = vi.fn();
		vi.stubGlobal("fetch", fetchMock);
		render(<CognitiveTree />);
		expect(screen.getByTestId("cognitive-tree-empty").textContent).toContain(
			"waiting for policy/current",
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

	it("shows the model problem as a banner with its exact reason and fix", () => {
		expect(ingestCognitionFrame("tree", mismatch)).toBe(true);
		render(<CognitiveTree />);
		const banner = screen.getByRole("alert");
		expect(banner.getAttribute("data-problem")).toBe("error");
		expect(banner.textContent).toContain(
			"paper trading inactive (model error)",
		);
		expect(banner.textContent).toContain(mismatch.problem.reason);
		expect(banner.textContent).toContain(
			"Fix: re-run detect and train on the tick epoch 3",
		);
	});

	it("shows no stored model as a warning banner", () => {
		ingestCognitionFrame("tree", {
			...mismatch,
			problem: {
				level: "warning",
				reason: "no trained policy is stored under policy/",
				fix: "run detect and train on a collected tick epoch",
			},
		});
		render(<CognitiveTree />);
		const banner = screen.getByRole("alert");
		expect(banner.getAttribute("data-problem")).toBe("warning");
		expect(banner.textContent).not.toContain("model error");
	});

	it("accepts a hub tree frame without gate or problem (PublishCognitionTree wire)", () => {
		const { gate: _g, problem: _p, ...hubTree } = tree;
		expect(ingestCognitionFrame("tree", hubTree)).toBe(true);
		render(<CognitiveTree />);
		expect(screen.queryByTestId("cognitive-tree-empty")).toBeNull();
		expect(screen.getByTestId("cognitive-tree-meta").textContent).toContain(
			"1 paths",
		);
	});

	it("refuses a tree frame whose problem is malformed, or missing", () => {
		const errors = vi.spyOn(console, "error").mockImplementation(() => {});
		expect(
			ingestCognitionFrame("tree", {
				...mismatch,
				problem: { level: "fatal", reason: "x", fix: "y" },
			}),
		).toBe(false);
		expect(
			ingestCognitionFrame("tree", {
				...mismatch,
				problem: { level: "error", reason: "x", fix: "" },
			}),
		).toBe(false);
		errors.mockRestore();
	});

	it("keeps the trie on the stored policy when live tokens move", () => {
		ingestPolicyLayers(1n, [
			{
				index: "policy/current/R01/R02/enter.bin",
				bytes: new Uint8Array([0]),
			},
			{
				index: "policy/current/R01/R02/exit.bin",
				bytes: new Uint8Array([0]),
			},
		]);
		ingestTrainRegions(
			9,
			"workspace/tick/aggregate/1/0000000000000000009.bin/train/regions/ETH/USD/R01",
			[0.5],
		);
		ingestTrainReject(
			9,
			"workspace/tick/aggregate/1/0000000000000000009.bin/train/reject/ETH/USD/R02",
		);
		ingestTrainRegions(
			10,
			"workspace/tick/aggregate/1/0000000000000000010.bin/train/regions/ETH/USD/R04/R08/R01",
			[0.25],
		);
		render(<CognitiveTree />);
		const meta = screen.getByTestId("cognitive-tree-meta").textContent ?? "";
		expect(meta).toContain("2 paths");
		expect(screen.getAllByText("R01").length).toBeGreaterThan(0);
		expect(screen.getAllByText("R02").length).toBeGreaterThan(0);
		expect(screen.queryByText("R04")).toBeNull();
		expect(screen.queryByText("R08")).toBeNull();
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

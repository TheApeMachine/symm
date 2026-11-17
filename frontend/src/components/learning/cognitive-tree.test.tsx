// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { bookAtom } from "#/collections/book";
import { cognitionStore } from "#/collections/cognition";
import { CognitiveTree, progressOf } from "./cognitive-tree";

vi.mock("#/providers/websocket", () => ({
	sendCommand: vi.fn(),
}));

afterEach(() => {
	cleanup();
	cognitionStore.setState(() => ({
		epoch: "0",
		policyKeys: [],
		thresholds: {},
		matches: {},
		problem: null,
		tree: null,
	}));
	vi.restoreAllMocks();
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
			"waiting for model/paths",
		);
		expect(fetchMock).not.toHaveBeenCalled();
		vi.unstubAllGlobals();
	});

	it("says paper trading is inactive when there are no policy keys", () => {
		bookAtom.set("paper");
		cognitionStore.setState((s) => ({
			...s,
			epoch: "1",
			tree: { trading: false, gate: null },
		}));
		render(<CognitiveTree />);
		expect(screen.getByTestId("cognitive-tree-empty").textContent).toContain(
			"no paper paths yet",
		);
	});

	it("shows the model problem banner from store state", () => {
		cognitionStore.setState((s) => ({
			...s,
			epoch: "1",
			policyKeys: [],
			problem: {
				level: "error",
				reason:
					"newest policy epoch 7 was trained under gate enabled=false; this run is configured for gate enabled=true",
				fix: "re-run detect and train on the tick epoch 3",
			},
		}));
		render(<CognitiveTree />);
		const banner = screen.getByRole("alert");
		expect(banner.getAttribute("data-problem")).toBe("error");
		expect(banner.textContent).toContain(
			"paper trading inactive (model error)",
		);
	});

	it("renders the trie when pathbook keys are loaded", () => {
		bookAtom.set("paper");
		cognitionStore.setState((s) => ({
			...s,
			epoch: "1792195368165650000",
			policyKeys: ["model/paths/R12/R10/enter.bin"],
			thresholds: { "model/paths/R12/R10/enter.bin": 0.45 },
			tree: {
				trading: true,
				gate: {
					tickEpoch: "1792189318815706000",
					enabled: true,
					surpriseZ: 1,
					warmup:
						"settled: error average at 0.15 no longer below its average at 0.01",
					learningRate: 0.01,
				},
			},
		}));
		render(<CognitiveTree />);
		const meta = screen.getByTestId("cognitive-tree-meta").textContent ?? "";
		expect(meta).toContain("policy epoch 1792195368165650000");
		expect(meta).toContain("1 paths");
		expect(meta).toContain(
			"gated z≥1 · warm-up settled: error average at 0.15 no longer below its average at 0.01",
		);
	});
});
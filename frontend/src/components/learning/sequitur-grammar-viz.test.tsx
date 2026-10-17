// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { focusAtom } from "#/collections/app";
import { PAPER_KEYS, type PaperRecord, paperStore } from "#/collections/paper";
import { ingestSequiturFrame, sequiturStore } from "#/collections/sequitur";
import { sequiturFrame } from "#/collections/sequitur-fixture";
import {
	buildNestedCapsules,
	frameToSnapshot,
	grammarResponseToSnapshot,
} from "./sequitur-engine";
import {
	describeEvent,
	ROOT_WINDOW,
	SequiturGrammarViz,
} from "./sequitur-grammar-viz";

describe("Sequitur Engine", () => {
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

		const snapshot = grammarResponseToSnapshot(mockResponse, []);

		expect(snapshot.rules.length).toBe(1);
		expect(snapshot.rules[0].id).toBe("M1");
		expect(snapshot.rules[0].action).toBe("enter");
		expect(snapshot.invariantStatus).toEqual({
			digramUniqueness: "NOMINAL",
			ruleUtility: "NOMINAL",
		});
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

		const snapshot = grammarResponseToSnapshot(mockResponse, []);
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

	it("builds the nested capsule hierarchy of a backend frame for the telescoping lens", () => {
		const snapshot = frameToSnapshot(sequiturFrame(), ROOT_WINDOW);
		const capsules = buildNestedCapsules(snapshot);

		expect(capsules.map((capsule) => capsule.symbol)).toEqual([
			"M1",
			"R05",
			"M1",
		]);
		expect(capsules[0].children?.map((child) => child.symbol)).toEqual([
			"R02",
			"R04",
		]);
		expect(capsules[1].isRule).toBe(false);
		expect(capsules[2]).toMatchObject({ startChar: 3, endChar: 4 });
	});
});

describe("frameToSnapshot", () => {
	it("windows S, keeps every rule and takes invariants from the backend check", () => {
		const start = Array.from({ length: 60 }, (_, i) => (i % 2 ? "M1" : "R05"));
		const snapshot = frameToSnapshot(
			sequiturFrame({
				start,
				violations: ["rule utility: M1 is referenced 1 time(s)"],
			}),
			ROOT_WINDOW,
		);

		expect(snapshot.rootRule.symbols).toHaveLength(ROOT_WINDOW);
		expect(snapshot.rules.map((rule) => rule.id)).toEqual(["M1"]);
		expect(snapshot.highlightRuleId).toBe("M1");
		expect(snapshot.invariantStatus).toEqual({
			digramUniqueness: "NOMINAL",
			ruleUtility: "VIOLATED",
		});
		expect(snapshot.title).toBe("Live Grammar: BTC/USD");
	});
});

describe("describeEvent", () => {
	it("names each Sequitur step with its rules", () => {
		const base = {
			token: undefined,
			rule: 3,
			into: 0,
			from: 2,
			digram: ["R01", "M2"] as [string, string],
		};
		expect(describeEvent({ ...base, kind: "append", token: "R07" })).toBe(
			"+ R07 appended to S",
		);
		expect(describeEvent({ ...base, kind: "create" })).toBe(
			"M3 ← R01 M2 (repeat in M2 and S)",
		);
		expect(describeEvent({ ...base, kind: "reuse" })).toBe(
			"R01 M2 is M3: substituted in S",
		);
		expect(describeEvent({ ...base, kind: "inline", into: 4 })).toBe(
			"M3 used once: inlined into M4",
		);
	});
});

describe("SequiturGrammarViz UI", () => {
	afterEach(() => {
		cleanup();
		sequiturStore.setState(() => ({}));
		focusAtom.set("BTC/USD");
	});

	it("claims nothing live before a frame arrives", () => {
		render(<SequiturGrammarViz />);

		expect(screen.getByText(/Sequitur Grammar/i)).toBeTruthy();
		expect(screen.getByText("awaiting frames")).toBeTruthy();
		expect(screen.queryByText("live")).toBeNull();
		expect(
			screen.getByText(/awaiting sequitur frames for BTC\/USD/),
		).toBeTruthy();
	});

	it("renders the gate, chord, steps, path and policy of the focused symbol's frame", () => {
		ingestSequiturFrame(sequiturFrame());
		render(<SequiturGrammarViz />);

		expect(screen.getByText("live")).toBeTruthy();
		expect(screen.getByTestId("gate-R02")).toBeTruthy();
		expect(screen.getByTestId("gate-R09")).toBeTruthy();
		expect(screen.getByTestId("active-path").textContent).toBe("R02/R04");
		expect(screen.getByText("M1 ← R02 R04 (repeat in S and S)")).toBeTruthy();
		expect(screen.getAllByText("enter").length).toBeGreaterThan(0);
		expect(screen.getByText("hold")).toBeTruthy();
		expect(
			screen.getByText(/policy epoch 1792187257188073000 · 12 paths/),
		).toBeTruthy();
	});

	it("shows what the surprise gate read and marks the chords it blocked", () => {
		ingestSequiturFrame(sequiturFrame());
		ingestSequiturFrame(
			sequiturFrame({
				tick: 42,
				fired: ["R09"],
				events: [],
				completed: "",
				gate: { surprise: 0.12, z: -0.4, warm: true, passed: false },
				blocked: true,
			}),
		);
		render(<SequiturGrammarViz />);

		const gate = screen.getByTestId("surprise-gate").textContent ?? "";
		expect(gate).toContain("blocked");
		expect(gate).toContain("surprise 0.120");
		expect(gate).toContain("z -0.40");
		expect(screen.getAllByTestId("chord-appended")).toHaveLength(1);
		expect(screen.getAllByTestId("chord-blocked")).toHaveLength(1);
	});

	it("names why a cold symbol's gate is closed", () => {
		ingestSequiturFrame(
			sequiturFrame({
				gate: {
					surprise: 0.9,
					z: null,
					warm: false,
					cold: "falling",
					passed: false,
				},
				blocked: true,
			}),
		);
		render(<SequiturGrammarViz />);

		expect(screen.getByTestId("surprise-gate").textContent).toContain(
			"cold: falling",
		);
	});

	it("says the gate is off when the frame carries none", () => {
		ingestSequiturFrame(sequiturFrame({ gate: null }));
		render(<SequiturGrammarViz />);

		expect(screen.getByTestId("surprise-gate").textContent).toContain(
			"gate off",
		);
	});

	it("says so when no policy is published or the path is not stored", () => {
		ingestSequiturFrame(
			sequiturFrame({
				policy: {
					epoch: "0",
					paths: 0,
					trading: false,
					stored: false,
					match: "",
					depth: 0,
					action: "",
					outcome: "",
				},
			}),
		);
		render(<SequiturGrammarViz />);
		expect(screen.getByText("no policy published")).toBeTruthy();
		expect(screen.getByText("none loaded")).toBeTruthy();
		expect(screen.getByText("inactive")).toBeTruthy();
	});

	it("shows the matched prefix, its depth and action, and what paper trading did", () => {
		ingestSequiturFrame(sequiturFrame());
		render(<SequiturGrammarViz />);
		expect(screen.getByTestId("matched-path").textContent).toBe("R02/R04");
		expect(screen.getByText("depth 2")).toBeTruthy();
		expect(screen.getByTestId("paper-outcome").textContent).toBe(
			"enter submitted",
		);
		expect(screen.getByText("no round trip closed yet")).toBeTruthy();
	});

	it("lists closed paper round trips", () => {
		ingestSequiturFrame(sequiturFrame());
		const fields = Object.fromEntries(
			PAPER_KEYS.map((key) => [key, 0]),
		) as PaperRecord["fields"];
		paperStore.setState(() => [
			{
				seq: 1,
				action: "exit",
				symbol: "BTC/USD",
				path: "R02/R04",
				policyEpoch: "7",
				fields: { ...fields, depth: 2, net_return: 0.0123, pnl: 1 },
			},
		]);
		render(<SequiturGrammarViz />);
		expect(screen.getByText("1 decision records")).toBeTruthy();
		expect(screen.getByText("R02/R04 · d2")).toBeTruthy();
		expect(screen.getByText("1.230%")).toBeTruthy();
		paperStore.setState(() => []);
	});

	it("shows backend invariant violations", () => {
		ingestSequiturFrame(
			sequiturFrame({
				violations: ["digram uniqueness: R02 R04 occurs in S and M1"],
			}),
		);
		render(<SequiturGrammarViz />);
		expect(screen.getByText("1 violated")).toBeTruthy();
		expect(
			screen.getByText("digram uniqueness: R02 R04 occurs in S and M1"),
		).toBeTruthy();
		expect(screen.getByText("Duplicate")).toBeTruthy();
	});

	it("shows only the focused symbol", () => {
		ingestSequiturFrame(sequiturFrame({ symbol: "ETH/USD" }));
		render(<SequiturGrammarViz />);
		expect(screen.getByText("awaiting frames")).toBeTruthy();
	});
});

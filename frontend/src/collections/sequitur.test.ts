import { afterEach, describe, expect, it, vi } from "vitest";
import { CHORD_LOG, ingestSequiturFrame, sequiturStore } from "./sequitur";
import { sequiturFrame } from "./sequitur-fixture";

afterEach(() => {
	sequiturStore.setState(() => ({}));
	vi.restoreAllMocks();
});

describe("ingestSequiturFrame", () => {
	it("keeps the latest frame and logs its events and chord per symbol", () => {
		expect(ingestSequiturFrame(sequiturFrame(), 1000)).toBe(true);
		expect(
			ingestSequiturFrame(
				sequiturFrame({ tick: 42, events: [], completed: "" }),
				2000,
			),
		).toBe(true);

		const feed = sequiturStore.state["BTC/USD"];
		expect(feed.frames).toBe(2);
		expect(feed.latest.tick).toBe(42);
		expect(feed.receivedAt).toBe(2000);
		expect(feed.events).toHaveLength(3);
		expect(feed.events.every((event) => event.tick === 41)).toBe(true);
		expect(feed.chords.map((chord) => chord.tick)).toEqual([41, 42]);
		expect(feed.chords[0]).toMatchObject({
			completed: "M1",
			stored: true,
			action: "enter",
		});
	});

	it("bounds the chord log", () => {
		for (let tick = 0; tick < CHORD_LOG + 10; tick++) {
			ingestSequiturFrame(sequiturFrame({ tick }));
		}
		const chords = sequiturStore.state["BTC/USD"].chords;
		expect(chords).toHaveLength(CHORD_LOG);
		expect(chords[chords.length - 1].tick).toBe(CHORD_LOG + 9);
	});

	it("reports and drops a payload that is not a frame", () => {
		const error = vi.spyOn(console, "error").mockImplementation(() => {});
		expect(ingestSequiturFrame({ symbol: "BTC/USD" })).toBe(false);
		expect(ingestSequiturFrame(undefined)).toBe(false);
		expect(error).toHaveBeenCalledTimes(2);
		expect(sequiturStore.state).toEqual({});
	});
});

describe("ingestSequiturFrame: backend grammar layer", () => {
	it("accepts a frame shaped like strategy.Model.publishGrammar", () => {
		expect(
			ingestSequiturFrame({
				symbol: "BTC/USD",
				epoch: "0",
				tick: 0,
				timestamp: "0",
				active: [{ key: 1, brightness: 0.8 }],
				midpoint: 0.8,
				fired: ["R01"],
				held: 1,
				gate: null,
				blocked: false,
				events: [
					{
						kind: "append",
						token: "R01",
						rule: 0,
						into: 0,
						from: 0,
						digram: ["", ""],
					},
				],
				completed: "",
				path: "R01",
				policy: {
					epoch: "0",
					paths: 0,
					trading: true,
					stored: false,
					match: "",
					depth: 0,
					action: "noop",
					outcome: "",
				},
				rules: [],
				start: ["R01"],
				rawLength: 1,
				grammarSymbols: 1,
				violations: [],
			}),
		).toBe(true);
	});
});

describe("ingestSequiturFrame: surprise gate", () => {
	it("refuses a frame whose gate reading is malformed", () => {
		const error = vi.spyOn(console, "error").mockImplementation(() => {});
		expect(
			ingestSequiturFrame({
				...sequiturFrame(),
				gate: { surprise: "high", z: null, warm: true, passed: true },
			}),
		).toBe(false);
		expect(
			ingestSequiturFrame({ ...sequiturFrame(), blocked: undefined }),
		).toBe(false);
		// A cold reading must name why; a warm one names no reason.
		expect(
			ingestSequiturFrame({
				...sequiturFrame(),
				gate: { surprise: 0.9, z: null, warm: false, passed: false },
			}),
		).toBe(false);
		expect(
			ingestSequiturFrame({
				...sequiturFrame(),
				gate: {
					surprise: 0.9,
					z: 1,
					warm: true,
					cold: "falling",
					passed: true,
				},
			}),
		).toBe(false);
		expect(error).toHaveBeenCalled();
		error.mockRestore();
	});

	it("logs the gate reading and blocked flag with each chord", () => {
		expect(
			ingestSequiturFrame(sequiturFrame({ symbol: "GATE/USD", blocked: true })),
		).toBe(true);
		const chord = sequiturStore.state["GATE/USD"].chords.at(-1);
		expect(chord?.blocked).toBe(true);
		expect(chord?.gate?.z).toBe(2.1);
	});
});

import { beforeEach, describe, expect, it } from "vitest";
import { cognitionStore } from "./cognition";
import { CHORD_LOG, ingestSequiturFrame, sequiturStore } from "./sequitur";
import { sequiturFrame } from "./sequitur-fixture";

beforeEach(() => {
	sequiturStore.setState(() => ({}));
	cognitionStore.setState(() => ({
		epoch: "0",
		policyKeys: [],
		thresholds: {},
		matches: {},
	}));
});

describe("ingestSequiturFrame", () => {
	it("appends chords and keeps the latest frame per symbol", () => {
		ingestSequiturFrame(sequiturFrame({ tick: 1 }), 1000);
		const live = sequiturFrame({ tick: 2, path: "R02/R04" });
		ingestSequiturFrame(live, 2000);

		const latest = sequiturStore.state["BTC/USD"].latest;
		expect(latest.tick).toBe(2);
		expect(latest.path).toBe("R02/R04");
	});

	it("rejects malformed payloads", () => {
		expect(ingestSequiturFrame(sequiturFrame(), 1000)).toBe(true);
		expect(ingestSequiturFrame({ symbol: "BTC/USD" }, 1000)).toBe(false);
		expect(ingestSequiturFrame(null, 1000)).toBe(false);
	});

	it("caps the chord log", () => {
		for (let tick = 0; tick < CHORD_LOG + 10; tick++) {
			ingestSequiturFrame(sequiturFrame({ tick }));
		}
		const chords = sequiturStore.state["BTC/USD"].chords;
		expect(chords).toHaveLength(CHORD_LOG);
		expect(chords[chords.length - 1].tick).toBe(CHORD_LOG + 9);
	});

	it("syncs cognition policy fields from the step frame", () => {
		ingestSequiturFrame(
			sequiturFrame({
				policy: {
					...sequiturFrame().policy,
					stored: true,
					match: "R02/R04/enter.bin",
					livePath: "R02/R04",
					policyPath: "R02/R04/enter.bin",
					outcome: "paper enter submitted",
				},
			}),
		);

		const match = cognitionStore.state.matches["BTC/USD"];
		expect(match?.stored).toBe(true);
		expect(match?.match).toBe("R02/R04/enter.bin");
		expect(match?.action).toBe("paper enter submitted");
		expect(match?.livePath).toBe("R02/R04");
	});
});

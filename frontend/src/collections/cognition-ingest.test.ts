import { describe, expect, it } from "vitest";
import {
	parseTrainRegionsIndex,
	ingestTrainRegions,
	cognitionStore,
	regionHistoryStore,
} from "./cognition";

/*
The backend emits one layer per region at
<prefix>/train/regions/<SYM>/<R##> with [brightness], and the live sentence
at <prefix>/train/regions/<SYM>/tokens with ["R01", ...]. These are exactly
those two shapes from a live BTC/USD frame.
*/
const PREFIX = "workspace/tick/aggregate/1792663234/0000000000000000075";

describe("training regions ingest", () => {
	it("feeds the ring from a region layer", () => {
		const ok = ingestTrainRegions(
			75,
			PREFIX + "/train/regions/BTC/USD/R01",
			[0.5],
		);
		expect(ok).toBe(true);
		const rings = regionHistoryStore.state["BTC/USD"];
		expect(rings).toBeDefined();
		expect(rings[rings.length - 1][1]).toBe(0.5);
	});

	it("feeds the match from the sentence layer", () => {
		ingestTrainRegions(
			75,
			PREFIX + "/train/regions/BTC/USD/tokens",
			["R01", "R02", "R03"],
		);
		const match = cognitionStore.state.matches["BTC/USD"];
		expect(match.tokens).toEqual(["R01", "R02", "R03"]);
		expect(match.depth).toBe(3);
	});

	it("parses both index shapes", () => {
		expect(
			parseTrainRegionsIndex(PREFIX + "/train/regions/BTC/USD/R07"),
		).toEqual({ symbol: "BTC/USD", tokens: ["R07"] });
		expect(
			parseTrainRegionsIndex(PREFIX + "/train/regions/BTC/USD/tokens"),
		).toEqual({ symbol: "BTC/USD", tokens: [] });
	});
});

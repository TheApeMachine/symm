import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { HindsightResident } from "./hindsight-types";
import { StatePanel } from "./inspector";

const resident: HindsightResident = {
	run: "run-1",
	symbol: "BTC/USD",
	sequence: 100,
	ordinal: 0,
	at: "2026-09-02T12:00:00Z",
	signals: [
		{
			source: "hawkes",
			identity: "hawkes-1",
			origin: {
				origin: {
					run: "run-1",
					sequence: 96,
					stream: "public:trade",
					streamEpoch: 1,
					streamSequence: 96,
				},
				ordinal: 0,
			},
			atNs: 1_788_336_000_000_000_000,
			ageNs: 25_000_000,
			hasAge: true,
			carried: true,
			maturity: 0.8,
			snr: 2.4,
			snrDefined: true,
			metrics: [
				{
					key: "conditional_intensity",
					raw: 1.75,
					normalized: 0,
					hasNormalized: false,
					standardized: 0,
					hasStandardized: false,
					unit: "/s",
				},
			],
		},
	],
	categories: [],
	perspectives: [],
	examined: 5,
	reachedBack: 4,
	exhausted: false,
	unresolved: [],
};

describe("StatePanel", () => {
	it("shows causally resident measurements when the exact envelope has no state witness", () => {
		const markup = renderToStaticMarkup(
			<StatePanel
				state={null}
				resident={resident}
				envelope={null}
				semantics={null}
			/>,
		);

		expect(markup).toContain("Resident state as-of this envelope");
		expect(markup).toContain("hawkes");
		expect(markup).toContain("reached back 4 captures");
		expect(markup).not.toContain("No exact or resident historical state");
	});
});

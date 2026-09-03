import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type {
	HindsightMetricMap,
	HindsightResident,
} from "./hindsight-types";
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

const semantics: HindsightMetricMap = {
	baselineCommit: "abc123",
	metrics: {
		"hawkes/conditional_intensity": {
			identity: "hawkes/conditional_intensity",
			source: "hawkes",
			metric: "conditional_intensity",
			role: "ARRIVAL_MODEL_STATE",
			class: "fitted_model_quantity",
			purpose: "Conditional arrival intensity of marked events.",
			forbidden: "Do not infer direction from intensity.",
		},
	},
	signals: {
		hawkes: {
			source: "hawkes",
			purpose:
				"The Hawkes signal measures the temporal arrival structure of marked market events.",
		},
	},
};

describe("MetricDetail through StatePanel", () => {
	it("states the estimator's own support in plain words, not a market verdict", () => {
		const markup = renderToStaticMarkup(
			<StatePanel
				state={null}
				resident={resident}
				envelope={null}
				semantics={semantics}
				plain={true}
			/>,
		);

		expect(markup).toContain("well supported");
		expect(markup).not.toMatch(/bullish|bearish|buy signal/i);
	});

	it("keeps the system's own vocabulary in expert mode", () => {
		const markup = renderToStaticMarkup(
			<StatePanel
				state={null}
				resident={resident}
				envelope={null}
				semantics={semantics}
				plain={false}
			/>,
		);

		expect(markup).toContain("mat");
		expect(markup).toContain("snr");
		expect(markup).not.toContain("well supported");
	});
});

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { HindsightMetricMap, Measurement } from "./hindsight-types";
import { StatePanel } from "./inspector";

const measurement: Measurement = {
	id: "meas-1",
	source: "hawkes",
	symbol: "BTC/USD",
	seqIdx: 100,
	at: "2026-09-02T12:00:00Z",
	maturity: 0.8,
	snr: 2.4,
	snrDefined: true,
	metrics: {
		conditional_intensity: {
			raw: 1.75,
			unit: "/s",
		},
	},
	peers: [
		{
			id: "peer-1",
			source: "cvd",
			symbol: "BTC/USD",
			seqIdx: 100,
			at: "2026-09-02T12:00:00Z",
			maturity: 0.9,
			snr: 3.1,
			snrDefined: true,
			metrics: {
				delta: {
					raw: 42.5,
				},
			},
		},
	],
};

describe("StatePanel", () => {
	it("shows observed measurements and concurrent peers", () => {
		const markup = renderToStaticMarkup(
			<StatePanel measurement={measurement} semantics={null} />,
		);

		expect(markup).toContain("Observed measurement at sequence 100");
		expect(markup).toContain("hawkes");
		expect(markup).toContain("1 concurrent peers");
		expect(markup).toContain("cvd");
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
				measurement={measurement}
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
				measurement={measurement}
				semantics={semantics}
				plain={false}
			/>,
		);

		expect(markup).toContain("mat");
		expect(markup).toContain("snr");
		expect(markup).not.toContain("well supported");
	});
});

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Measurement } from "#/collections/types";
import { MeasurementInspection } from "#/components/kernel/measurement-inspection";

describe("MeasurementInspection", () => {
	it("renders every metric and the complete measurement provenance", () => {
		const measurement = {
			id: "depthflow:BTC/USD",
			source: "depthflow",
			symbol: "BTC/USD",
			peer: "ETH/USD",
			at: "2026-08-10T12:39:28Z",
			observedFrom: "2026-08-10T12:39:27Z",
			horizon: 1_000_000_000,
			maturity: 0.997,
			uncertainty: {
				lower: 0.91,
				upper: 1.03,
				confidence: 0.95,
				method: "bootstrap",
			},
			metrics: {
				strength: {
					raw: 0.9928,
					normalized: 0.9928,
					unit: "dimensionless",
				},
				spoof_score: {
					raw: -0.14,
					normalized: -0.2,
					unit: "dimensionless",
				},
			},
			validity: { state: "valid", readiness: "observation" },
			scale: { kind: "", from: "", through: "" },
		} satisfies Measurement;

		const markup = renderToStaticMarkup(
			<MeasurementInspection measurement={measurement} />,
		);

		expect(markup).toContain("depthflow:BTC/USD");
		expect(markup).toContain("2026-08-10 12:39:27.000 UTC");
		expect(markup).toContain("1000000000 ns");
		expect(markup).toContain("bootstrap");
		expect(markup).toContain("strength");
		expect(markup).toContain("spoof score");
		expect(markup).toContain("0.9928");
		expect(markup).toContain("-0.2");
		expect(markup).toContain("2 readings");
	});

	it("states when the focused symbol has not been observed", () => {
		const markup = renderToStaticMarkup(
			<MeasurementInspection measurement={null} />,
		);

		expect(markup).toContain(
			"No measurement has been observed for this symbol.",
		);
	});
});

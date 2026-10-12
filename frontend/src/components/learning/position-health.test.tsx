import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import {
	PositionHealthGauge,
	LiveRegionActivationMap,
	RegionTransitionPathway,
	ActivePositionHealthCard,
	TradeRegionScores,
	PositionRegionMonitor,
	REGIONS,
} from "./position-health";
import type { Lifecycle } from "#/lib/lifecycle";
import { RingBuffer, signals } from "#/collections/app";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";

describe("position-health components", () => {
	const mockLife: Lifecycle = {
		id: "pos-1",
		symbol: "ZAMA/USD",
		status: "open",
		opened_at: new Date().toISOString(),
		events: [
			{
				at: new Date().toISOString(),
				kind: "fill",
				detail: "buy fill",
				fields: {
					cost: 39.55,
					fee: 0.3164,
					volume: 517.71,
					price: 0.07639,
				},
			},
		],
	};

	it("renders PositionHealthGauge with vitality percentage and health verdict", () => {
		const markup = renderToStaticMarkup(
			<PositionHealthGauge life={mockLife} unrealizedVal={-0.32} />,
		);
		expect(markup).toContain("Position Health Gauge");
		expect(markup).toContain("Vitality");
		expect(markup).toContain("$39.55");
		expect(markup).toContain("-0.32 USD");
	});

	it("renders LiveRegionActivationMap highlighting the active token and visited regions", () => {
		const markup = renderToStaticMarkup(
			<LiveRegionActivationMap currentPath="R01/R04/R06" symbol="ZAMA/USD" />,
		);
		expect(markup).toContain("Live Region Confluence Radar");
		expect(markup).toContain("R06 ACTIVE");
		expect(markup).toContain("R06");
		expect(markup).toContain("R01");
		expect(markup).toContain("R04");
	});

	it("renders RegionTransitionPathway with entry sequence and exit target rules", () => {
		const markup = renderToStaticMarkup(
			<RegionTransitionPathway
				entryPath="R01/R04/R06"
				currentPath="R01/R04/R06"
			/>
		);
		expect(markup).toContain("Entry Ignition Pathway");
		expect(markup).toContain("★ ENTER");
		expect(markup).toContain("Active Exit Targets");
		expect(markup).toContain("⚡ EXIT");
		expect(markup).toContain("R11 (Cohort Drag)");
	});

	it("renders ActivePositionHealthCard for open position without fake realized loss", () => {
		const markup = renderToStaticMarkup(
			<ActivePositionHealthCard life={mockLife} />,
		);
		expect(markup).toContain("OPEN POSITION");
		expect(markup).toContain("Holding 517.71 ZAMA");
		expect(markup).toContain("4. HOLD (Protected)");
		expect(markup).not.toContain("Realized");
	});

	it("renders TradeRegionScores with live measured brightness from signals.training", () => {
		const ring = new RingBuffer<MeasurementT>(10);
		ring.add({
			symbol: "ZAMA/USD",
			source: "training",
			metrics: [
				{ name: "region_brightness:R06", raw: 2.85 },
				{ name: "region_members:R06", raw: 7 },
				{ name: "region_brightness:R01", raw: 1.42 },
				{ name: "region_members:R01", raw: 4 },
			],
			provenance: [
				{ name: "token_path", value: "R01/R06" },
			],
		} as unknown as MeasurementT);
		signals.training.state["ZAMA/USD"] = ring;

		const markup = renderToStaticMarkup(
			<TradeRegionScores life={mockLife} />,
		);
		expect(markup).toContain("R06");
		expect(markup).toContain("R00");
		expect(markup).toContain("2.85 σ");
		expect(markup).toContain("ACTIVE TRIGGER");
		expect(markup).toContain("7 metrics");
		expect(markup).toContain("1.42 σ");
		expect(markup).toContain("PATH COMPONENT");
	});

	it("renders PositionRegionMonitor with default Position & Region Radar tab", () => {
		const markup = renderToStaticMarkup(
			<PositionRegionMonitor life={mockLife} />,
		);
		expect(markup).toContain("Position &amp; Region Radar");
		expect(markup).toContain("Decision Stream");
		expect(markup).toContain("Position Health Gauge");
	});

	it("has complete region metadata for R00 through R11", () => {
		for (let i = 0; i <= 11; i++) {
			const key = `R${i.toString().padStart(2, "0")}`;
			expect(REGIONS[key]).toBeDefined();
			expect(REGIONS[key].title).toBeTruthy();
			expect(REGIONS[key].description).toBeTruthy();
		}
	});
});

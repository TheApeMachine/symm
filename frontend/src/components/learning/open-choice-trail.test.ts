import { describe, expect, it } from "vitest";
import {
	CHOICE_TRAIL_LENGTH,
	indexMean,
	linePath,
	noteOpenChoice,
	officialExitDashIndex,
	clipTraceToValues,
	openChoiceFloorBands,
	openChoiceFloors,
	openChoiceSeries,
	openChoiceTrail,
	openChoiceValues,
	resetOpenChoices,
} from "./open-choice-trail";

describe("open choice trail", () => {
	it("skips duplicate marks so flat ticks do not grow the buffer", () => {
		resetOpenChoices();
		noteOpenChoice("doge", -1.5, "mark-a");
		noteOpenChoice("doge", -1.5, "mark-a");
		expect(openChoiceTrail("doge")).toHaveLength(1);
		noteOpenChoice("doge", -1.5, "mark-b");
		expect(openChoiceTrail("doge")).toHaveLength(2);
	});

	it("exposes the same values array the card Trail uses", () => {
		resetOpenChoices();
		noteOpenChoice("doge", 1.2, "a");
		noteOpenChoice("doge", -0.5, "b");
		expect(openChoiceValues("doge")).toEqual([1.2, -0.5]);
	});

	it("joins samples in order the way the card trail does", () => {
		expect(
			linePath([
				{ x: 0, y: 1 },
				{ x: 10, y: 4 },
			]),
		).toBe("M0.0,1.0 L10.0,4.0");
	});

	it("right-aligns a shorter lot so now stays the last slot", () => {
		expect(
			indexMean([
				[
					{ t: 1, pnl: 1, stamp: "a" },
					{ t: 2, pnl: 3, stamp: "b" },
				],
				[{ t: 2, pnl: 5, stamp: "c" }],
			]),
		).toEqual([1, 4]);
	});

	it("caps the ring buffer at CHOICE_TRAIL_LENGTH", () => {
		resetOpenChoices();
		for (let i = 0; i < CHOICE_TRAIL_LENGTH + 5; i++) {
			noteOpenChoice("doge", i, `tick-${i}`);
		}
		expect(openChoiceTrail("doge")).toHaveLength(CHOICE_TRAIL_LENGTH);
		expect(openChoiceTrail("doge")[0].pnl).toBe(5);
	});

	it("dashes after official exit only when the adaptive shadow is beating it", () => {
		expect(
			officialExitDashIndex({
				official_exit_at: "2026-01-01T00:00:00Z",
				official_exit_pct: 0.2,
				pnl_trace: [0.1, 0.4, 0.2],
				shadow_pnl_trace: [0.1, 0.4, 0.2, 0.5, 0.8],
			}),
		).toBe(3);

		expect(
			officialExitDashIndex({
				official_exit_at: "2026-01-01T00:00:00Z",
				official_exit_pct: 0.2,
				pnl_trace: [0.1, 0.4, 0.2],
				shadow_pnl_trace: [0.1, 0.4, 0.2, 0.15, 0.05],
			}),
		).toBeUndefined();

		expect(
			officialExitDashIndex({
				pnl_trace: [0.1, 0.4],
				shadow_pnl_trace: [0.1, 0.4, 0.2],
			}),
		).toBeUndefined();

		expect(
			officialExitDashIndex({
				official_exit_at: "2026-01-01T00:00:00Z",
				official_exit_pct: 0.2,
				pnl_trace: [0.1, 0.4, 0.2],
				shadow_pnl_trace: [0.1, 0.4, 0.2],
			}),
		).toBeUndefined();
	});

	it("keeps the adaptive path dashed when it beats; otherwise stays on official", () => {
		resetOpenChoices();
		const beating = openChoiceSeries([
			{
				id: "lot-1",
				symbol: "DOGE/USD",
				official_exit_at: "2026-01-01T00:00:00Z",
				official_exit_pct: 0.5,
				pnl_trace: [0, 1, 0.5],
				shadow_pnl_trace: [0, 1, 0.5, 0.9, 1.2],
				shadow_active: true,
			},
		]);

		expect(beating).toHaveLength(1);
		expect(beating[0].values).toEqual([0, 1, 0.5, 0.9, 1.2]);
		expect(beating[0].dashFrom).toBe(3);
		expect(beating[0].shadowActive).toBe(true);

		const losing = openChoiceSeries([
			{
				id: "lot-2",
				symbol: "DOGE/USD",
				official_exit_at: "2026-01-01T00:00:00Z",
				official_exit_pct: 0.5,
				pnl_trace: [0, 1, 0.5],
				shadow_pnl_trace: [0, 1, 0.5, 0.4, 0.2],
				shadow_active: true,
			},
		]);

		expect(losing[0].values).toEqual([0, 1, 0.5]);
		expect(losing[0].dashFrom).toBeUndefined();
	});

	it("reads official and structural floors for chart guides", () => {
		expect(
			openChoiceFloors({
				stop_floor_pct: -1.8,
				shadow_floor_pct: 0.2,
			}),
		).toEqual({ stopFloor: -1.8, shadowFloor: 0.2 });

		expect(
			openChoiceFloors({
				stop_state: { floorPct: -2 },
			}),
		).toEqual({ stopFloor: -2, shadowFloor: undefined });

		const series = openChoiceSeries([
			{
				id: "lot-2",
				symbol: "ETH/USD",
				pnl_trace: [0, 0.5],
				shadow_pnl_trace: [0, 0.5],
				stop_floor_pct: -2,
				shadow_floor_pct: 0.2,
				stop_floor_trace: [-2, -1.5],
				shadow_floor_trace: [-2, 0.2],
				pnl: 1.25,
			},
		]);
		expect(series[0].stopFloor).toBe(-2);
		expect(series[0].shadowFloor).toBe(0.2);
		expect(series[0].stopFloorTrace).toEqual([-2, -1.5]);
		expect(series[0].shadowFloorTrace).toEqual([-2, 0.2]);
		expect(series[0].pnlMoney).toBe(1.25);
	});

	it("clips floor traces to the visible value window", () => {
		expect(clipTraceToValues([-2, -1, 0, 0.2, 0.5], 3, 5)).toEqual([
			0, 0.2, 0.5,
		]);

		expect(
			openChoiceFloorBands(
				{
					stop_floor_trace: [-2, -1, 0.2],
					shadow_pnl_trace: [0, 1, 2],
					pnl_trace: [0, 1, 2],
				},
				[0, 1, 2],
			).stopFloorTrace,
		).toEqual([-2, -1, 0.2]);
	});
});

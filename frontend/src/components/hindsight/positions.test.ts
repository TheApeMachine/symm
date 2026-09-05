import { describe, expect, it } from "vitest";
import type { HindsightLifecycleEvent } from "./hindsight-types";
import { buildPositions, positionsFor } from "./positions";

const fill = (
	kind: string,
	at: string,
	avgPrice?: string,
	fee?: string,
): HindsightLifecycleEvent => ({
	decisionId: "d1",
	symbol: "POND/USD",
	kind,
	action: "",
	at,
	execution:
		avgPrice === undefined
			? null
			: {
					orderId: "PAPER-1",
					execId: "PAPER-2",
					side: kind === "entry_fill" ? "buy" : "sell",
					orderStatus: "filled",
					avgPrice,
					cumQty: "100",
					feeUsdEquiv: fee,
					fillAt: at,
				},
});

describe("buildPositions", () => {
	it("folds a round trip into one position keyed by its decision", () => {
		const [position] = buildPositions([
			fill("entry_fill", "2026-08-30T20:22:14Z", "0.000959", "0.17"),
			{ ...fill("position_open", "2026-08-30T20:22:14Z"), action: "enter" },
			fill("exit_fill", "2026-08-30T20:24:54Z", "0.001139", "0.18"),
			fill("position_close", "2026-08-30T20:24:54Z"),
		]);

		expect(position.decisionId).toBe("d1");
		expect(position.entry?.price).toBeCloseTo(0.000959);
		expect(position.exit?.price).toBeCloseTo(0.001139);
		expect(position.open).toBe(false);
		expect(position.realisedPriceChange).toBeCloseTo(0.1877, 4);
		expect(position.fees).toBeCloseTo(0.35);
	});

	it("leaves a position with no recorded exit open", () => {
		const [position] = buildPositions([
			fill("entry_fill", "2026-08-30T20:22:14Z", "0.000959"),
		]);

		expect(position.open).toBe(true);
		expect(position.exit).toBeNull();
		expect(position.realisedPriceChange).toBeNull();
	});

	it("leaves the realised change undefined when a fill carried no price", () => {
		const [position] = buildPositions([
			fill("entry_fill", "2026-08-30T20:22:14Z"),
			fill("exit_fill", "2026-08-30T20:24:54Z", "0.001139"),
		]);

		expect(position.entry).toBeNull();
		expect(position.realisedPriceChange).toBeNull();
	});

	it("reports fees as unreported rather than as zero", () => {
		const [position] = buildPositions([
			fill("entry_fill", "2026-08-30T20:22:14Z", "0.000959"),
			fill("exit_fill", "2026-08-30T20:24:54Z", "0.001139"),
		]);

		expect(position.fees).toBeNull();
	});

	it("keeps separate decisions apart and narrows to one instrument", () => {
		const other: HindsightLifecycleEvent = {
			...fill("entry_fill", "2026-08-30T20:30:00Z", "17.6"),
			decisionId: "d2",
			symbol: "VVV/USD",
		};

		const positions = buildPositions([
			fill("entry_fill", "2026-08-30T20:22:14Z", "0.000959"),
			other,
		]);

		expect(positions).toHaveLength(2);
		expect(positionsFor(positions, "POND/USD")).toHaveLength(1);
		expect(positionsFor(positions, "VVV/USD")[0].decisionId).toBe("d2");
	});
});

const event = (
	decisionId: string,
	symbol: string,
	kind: string,
): HindsightLifecycleEvent => ({
	decisionId,
	symbol,
	kind,
	action: "",
	at: "2026-09-04T07:50:37.265417Z",
	execution: null,
});

describe("seeking a position back to its frame", () => {
	it("carries the frame the causing decision was recorded on", () => {
		const positions = buildPositions([
			{
				...event("d1", "SOL/USD", "entry_fill"),
				captureSeq: 1140986,
				execution: {
					orderId: "P1",
					clientOrderId: "d1",
					execId: "E1",
					side: "buy",
					orderStatus: "filled",
					avgPrice: "103.72",
					cumQty: "0.19",
					feeUsdEquiv: "0.1585",
					fillAt: "2026-09-04T07:50:37.265417Z",
				},
			},
		]);

		expect(positions).toHaveLength(1);
		expect(positions[0].entrySeq).toBe(1140986);
	});

	it("stays unseekable rather than guessing when no frame was recorded", () => {
		const positions = buildPositions([event("d2", "MON/USD", "position_open")]);

		expect(positions[0].entrySeq).toBeNull();
	});

	it("ignores a zero frame, which means the join found nothing", () => {
		const positions = buildPositions([
			{ ...event("d3", "AVA/USD", "position_open"), captureSeq: 0 },
		]);

		expect(positions[0].entrySeq).toBeNull();
	});
});

describe("the exit edge", () => {
	it("takes a stamped exit frame from the record", () => {
		const positions = buildPositions([
			{ ...event("d4", "SOL/USD", "position_open"), captureSeq: 100 },
			{ ...event("d4", "SOL/USD", "position_close"), captureSeq: 460 },
		]);

		expect(positions[0].entrySeq).toBe(100);
		expect(positions[0].exitSeq).toBe(460);
	});

	it("never borrows the entry frame for an unstamped exit", () => {
		const positions = buildPositions([
			{ ...event("d5", "SOL/USD", "position_open"), captureSeq: 100 },
			{ ...event("d5", "SOL/USD", "position_close"), captureSeq: 0 },
		]);

		expect(positions[0].entrySeq).toBe(100);
		expect(positions[0].exitSeq).toBeNull();
	});
});

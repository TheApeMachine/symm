import { afterEach, describe, expect, it, vi } from "vitest";
import {
	abortDetections,
	beginDetectionRuns,
	beginDetections,
	beginDetectionTape,
	DETECTION_RUN_KEYS,
	detectionsStore,
	ingestDetectionFrame,
} from "./detections";
import {
	EXCURSION_KEYS,
	HINDSIGHT_DONE_KEYS,
	type HindsightFrame,
} from "./hindsight";

const RUN = 1792189318815706000n;

const vector = (values: number[]) => {
	const bytes = new Uint8Array(values.length * 8);
	const view = new DataView(bytes.buffer);
	values.forEach((value, idx) => {
		view.setFloat64(idx * 8, value, true);
	});
	return bytes;
};

const frame = (
	role: string,
	scope: string,
	layers: { index: string; bytes: Uint8Array }[],
	epoch = RUN,
): HindsightFrame => ({ epoch, tick: 0n, timestamp: 0n, role, scope, layers });

const keyed = (key: string, labels: readonly string[], values: number[]) => ({
	index: `${key}.bin/${labels.join("/")}`,
	bytes: vector(values),
});

const done = (scope: string, epoch = RUN) =>
	frame(
		"done",
		scope,
		[
			keyed(
				"done",
				HINDSIGHT_DONE_KEYS,
				HINDSIGHT_DONE_KEYS.map(() => 0),
			),
		],
		epoch,
	);

afterEach(() => {
	detectionsStore.setState(() => ({
		runs: null,
		excursions: null,
		tape: null,
	}));
	vi.restoreAllMocks();
});

describe("detections", () => {
	it("streams the runs, an epoch's excursions and an excursion's trades into their slots", () => {
		expect(beginDetectionRuns()).toEqual({ type: "detections" });
		ingestDetectionFrame(
			frame("detection_run", "", [
				keyed(
					`detector/watermark/all/${RUN}/9`,
					DETECTION_RUN_KEYS,
					[1, 9, 9, 1, 1, 0, 0, 0],
				),
			]),
		);
		ingestDetectionFrame(done("", 0n));
		expect(detectionsStore.state.runs).toMatchObject({
			status: "done",
			items: [{ epoch: RUN.toString(), fields: { ticks: 9, up: 1 } }],
		});

		expect(beginDetections(RUN.toString())).toEqual({
			type: "detections",
			run: RUN.toString(),
		});
		const fields = EXCURSION_KEYS.map((_, idx) => idx);
		ingestDetectionFrame(
			frame("detector", "BTC/USD", [
				keyed(
					`detector/up/BTC/USD/${RUN}/${"9".padStart(19, "0")}`,
					EXCURSION_KEYS,
					fields,
				),
			]),
		);
		ingestDetectionFrame(done(""));
		expect(detectionsStore.state.excursions).toMatchObject({
			run: RUN.toString(),
			status: "done",
			items: [
				{ symbol: "BTC/USD", class: "up", fields: { a_idx: 0, b_idx: 2 } },
			],
		});

		expect(beginDetectionTape(RUN.toString(), "BTC/USD", "up", "9")).toEqual({
			type: "detections",
			run: RUN.toString(),
			symbol: "BTC/USD",
			class: "up",
			at: "9",
		});
		const tape = `workspace/tick/aggregate/${RUN}/2`;
		ingestDetectionFrame(
			frame("trades", "BTC/USD", [
				keyed(tape, ["seq"], [7, 8]),
				keyed(tape, ["price"], [100, 101]),
				keyed(tape, ["at"], [1e18, 2e18]),
			]),
		);
		ingestDetectionFrame(done("BTC/USD"));
		expect(detectionsStore.state.tape).toMatchObject({
			status: "done",
			items: [
				{ seq: 7, price: 100, at: 1e18 },
				{ seq: 8, price: 101, at: 2e18 },
			],
		});
		// The excursions stay listed while a tape streams.
		expect(detectionsStore.state.excursions?.items).toHaveLength(1);
	});

	it("drops frames of a request since replaced", () => {
		beginDetections("7");
		ingestDetectionFrame(
			frame(
				"detector",
				"BTC/USD",
				[
					keyed(
						"detector/up/BTC/USD/8/9",
						EXCURSION_KEYS,
						EXCURSION_KEYS.map(() => 0),
					),
				],
				8n,
			),
		);
		ingestDetectionFrame(done("", 7n));
		expect(detectionsStore.state.excursions).toMatchObject({
			run: "7",
			status: "done",
			items: [],
		});
	});

	it("fails a stream loudly on a malformed frame or the hub's error frame", () => {
		const logged = vi.spyOn(console, "error").mockImplementation(() => {});

		beginDetectionRuns();
		ingestDetectionFrame(
			frame("detection_run", "", [
				keyed("detector/watermark/all/7/9", ["ticks"], [9]),
			]),
		);
		expect(detectionsStore.state.runs?.status).toBe("error");
		expect(detectionsStore.state.runs?.error).toContain(
			"malformed detection_run layer",
		);

		beginDetectionTape("7", "BTC/USD", "up", "9");
		ingestDetectionFrame(
			frame(
				"error",
				"BTC/USD",
				[
					{
						index: "error",
						bytes: new TextEncoder().encode("tick 2 holds no trade at b_at"),
					},
				],
				7n,
			),
		);
		expect(detectionsStore.state.tape).toMatchObject({
			status: "error",
			error: "tick 2 holds no trade at b_at",
		});
		expect(logged).toHaveBeenCalled();
	});

	it("abortDetections fails only the streams in flight", () => {
		const logged = vi.spyOn(console, "error").mockImplementation(() => {});

		abortDetections("idle");
		expect(logged).not.toHaveBeenCalled();

		beginDetectionRuns();
		ingestDetectionFrame(done("", 0n));
		beginDetections("7");
		abortDetections("hub socket closed");

		expect(detectionsStore.state.runs?.status).toBe("done");
		expect(detectionsStore.state.excursions).toMatchObject({
			status: "error",
			error: "hub socket closed",
		});
		expect(logged).toHaveBeenCalledWith(
			"detections stream failed:",
			"hub socket closed",
		);
	});
});

import * as flatbuffers from "flatbuffers";
import { renderToStaticMarkup } from "react-dom/server";
import { afterAll, beforeEach, describe, expect, it } from "vitest";
import { strategyStore } from "#/collections/app";
import { DecisionsSurface } from "#/components/terminal/decisions-surface";
import { DecisionT } from "#/providers/telemetry/telemetry/decision";
import {
	StrategyFrame,
	StrategyFrameT,
} from "#/providers/telemetry/telemetry/strategy-frame";

/*
frameFor builds a strategy frame carrying one symbol's decision, which is what
the backend actually emits: a frame is per-tick and usually names one symbol.
*/
const frameFor = (symbol: string): StrategyFrame => {
	const builder = new flatbuffers.Builder(1024);
	const frameT = new StrategyFrameT(true, "admission", [
		new DecisionT(`d-${symbol}`, "nothing", symbol),
	]);
	builder.finish(frameT.pack(builder));

	return StrategyFrame.getRootAsStrategyFrame(
		new flatbuffers.ByteBuffer(builder.asUint8Array()),
	);
};

describe("DecisionsSurface", () => {
	beforeEach(() => {
		strategyStore.actions.reset();
	});

	afterAll(() => {
		strategyStore.actions.reset();
	});

	it("waits explicitly when no frames have arrived", () => {
		expect(renderToStaticMarkup(<DecisionsSurface />)).toContain(
			"waiting for backend decision frames",
		);
	});

	it("keeps a row per symbol instead of mirroring the newest frame", () => {
		// Consecutive frames naming different symbols previously replaced the
		// single visible row each tick, so a decision appeared and was
		// immediately overwritten.
		strategyStore.actions.add(frameFor("SKR/USD"));
		strategyStore.actions.add(frameFor("BTC/USD"));

		const markup = renderToStaticMarkup(<DecisionsSurface />);

		expect(markup).toContain("SKR/USD");
		expect(markup).toContain("BTC/USD");
	});

	it("keeps one row per symbol when a symbol is decided repeatedly", () => {
		strategyStore.actions.add(frameFor("SKR/USD"));
		strategyStore.actions.add(frameFor("SKR/USD"));
		strategyStore.actions.add(frameFor("SKR/USD"));

		const markup = renderToStaticMarkup(<DecisionsSurface />);
		const occurrences = markup.split("SKR/USD").length - 1;

		// The symbol paints in its row once; repeated decisions update that
		// row rather than stacking duplicates.
		expect(occurrences).toBeLessThanOrEqual(2);
	});
});

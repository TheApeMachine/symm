import * as flatbuffers from "flatbuffers";
import { renderToStaticMarkup } from "react-dom/server";
import { afterAll, beforeEach, describe, expect, it } from "vitest";
import { strategyStore } from "#/collections/app";
import { DecisionT } from "#/providers/telemetry/telemetry/decision";
import { StrategyFrame, StrategyFrameT } from "#/providers/telemetry/telemetry/strategy-frame";
import { DecisionChain } from "./decision-chain";

const createMockStrategyFrame = (): StrategyFrame => {
	const builder = new flatbuffers.Builder(1024);
	const frameT = new StrategyFrameT(true, "decisions", [
		new DecisionT("d1", "enter", "BTC/USD"),
	]);
	const offset = frameT.pack(builder);
	builder.finish(offset);
	return StrategyFrame.getRootAsStrategyFrame(
		new flatbuffers.ByteBuffer(builder.asUint8Array()),
	);
};

describe("DecisionChain", () => {
	beforeEach(() => {
		strategyStore.actions.reset();
		strategyStore.actions.add(createMockStrategyFrame());
	});

	afterAll(() => {
		strategyStore.actions.reset();
	});


	it("starts compact while retaining the full structural decision trace", () => {
		const markup = renderToStaticMarkup(<DecisionChain index={0} />);

		expect(markup).toContain('aria-expanded="false"');
		expect(markup).toContain("group-data-[selected=true]:grid");
		expect(markup).toContain("1 · structural thesis");
		expect(markup).toContain("2 · evidence graph");
		expect(markup).toContain("3 · graph search");
		expect(markup).toContain("4 · execution + risk");
		expect(markup).toContain('data-df="symbol"');
		expect(markup).toContain('data-df="thesisScore"');
		expect(markup).toContain('data-df="thesisConfidence"');
		expect(markup).toContain('data-df="graphScore"');
		expect(markup).toContain('data-df="action"');
		expect(markup).not.toContain("edge=");
	});
});

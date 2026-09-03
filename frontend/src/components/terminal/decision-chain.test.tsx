import * as flatbuffers from "flatbuffers";
import { renderToStaticMarkup } from "react-dom/server";
import { afterAll, beforeEach, describe, expect, it } from "vitest";
import { strategyStore } from "#/collections/app";
import { DecisionT } from "#/providers/telemetry/telemetry/decision";
import { DecisionTraceT } from "#/providers/telemetry/telemetry/decision-trace";
import { MCTSNodeT } from "#/providers/telemetry/telemetry/mctsnode";
import {
	StrategyFrame,
	StrategyFrameT,
} from "#/providers/telemetry/telemetry/strategy-frame";
import { DecisionChain } from "./decision-chain";

/*
mockTrace mirrors a real search result: a root with a selected entry branch and
a pruned sibling carrying counterfactual mass.
*/
const mockTrace = (): DecisionTraceT => {
	const trace = new DecisionTraceT();
	trace.recommendedAction = "enter";
	trace.iterations = BigInt(24);
	trace.horizon = BigInt(5);
	trace.transitionSource = "war-room-consensus";
	trace.identificationStatus = "identified";
	trace.consensusDominantMove = "explosive_pump";
	trace.consensusConfidence = 0.706;
	trace.consensusParticipants = BigInt(3);

	const root = new MCTSNodeT();
	root.actionName = "root";
	root.visits = BigInt(24);
	root.effectiveVisits = 24;

	const enter = new MCTSNodeT();
	enter.actionName = "enter";
	enter.depth = BigInt(1);
	enter.visits = BigInt(23);
	enter.effectiveVisits = 23;
	enter.selected = true;

	root.children = [enter];
	trace.tree = root;

	return trace;
};

const createMockStrategyFrame = (): StrategyFrame => {
	const builder = new flatbuffers.Builder(1024);
	const decision = new DecisionT("d1", "enter", "BTC/USD");
	decision.trace = mockTrace();
	const frameT = new StrategyFrameT(true, "decisions", [decision]);
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
		const markup = renderToStaticMarkup(<DecisionChain frame={0} index={0} />);

		expect(markup).toContain('aria-expanded="false"');
		expect(markup).toContain("group-data-[selected=true]:grid");
		expect(markup).toContain("1 · structural thesis");
		expect(markup).toContain("2 · evidence graph");
		expect(markup).toContain("3 · war room");
		expect(markup).toContain("4 · causal search");
		expect(markup).toContain("5 · execution + risk");
		expect(markup).toContain('data-df="symbol"');
		expect(markup).toContain('data-df="thesisScore"');
		expect(markup).toContain('data-df="thesisConfidence"');
		expect(markup).toContain('data-df="graphScore"');
		expect(markup).toContain('data-df="action"');
		expect(markup).not.toContain("edge=");
	});

	it("renders the live search tree through the chain", () => {
		const markup = renderToStaticMarkup(<DecisionChain frame={0} index={0} />);

		// The stage must receive the real telemetry decision, not a stripped
		// projection: passing the thesis object silently drops the trace and
		// renders "no search this round" forever.
		expect(markup).toContain("war-room-consensus");
		expect(markup).toContain("explosive_pump");
		expect(markup).not.toContain("no search this round");
	});

	it("paints the row from the frame already in hand", () => {
		const markup = renderToStaticMarkup(<DecisionChain frame={0} index={0} />);

		// The row's fields are painted imperatively by the store subscription;
		// without seeding they render as an empty outline until the next frame.
		expect(markup).toContain("BTC/USD");
	});
});

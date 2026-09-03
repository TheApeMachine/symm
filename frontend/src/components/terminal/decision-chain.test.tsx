import { renderToStaticMarkup } from "react-dom/server";
import { afterAll, beforeEach, describe, expect, it } from "vitest";
import { decisionStore } from "#/collections/app";
import { DecisionT } from "#/providers/telemetry/telemetry/decision";
import { DecisionTraceT } from "#/providers/telemetry/telemetry/decision-trace";
import { MCTSNodeT } from "#/providers/telemetry/telemetry/mctsnode";
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

const mockDecision = (): DecisionT => {
	const decision = new DecisionT("d1", "enter", "BTC/USD");
	decision.trace = mockTrace();

	return decision;
};

describe("DecisionChain", () => {
	beforeEach(() => {
		decisionStore.actions.reset();
		decisionStore.actions.add(mockDecision());
	});

	afterAll(() => {
		decisionStore.actions.reset();
	});

	it("starts compact while retaining the full structural decision trace", () => {
		const markup = renderToStaticMarkup(<DecisionChain symbol="BTC/USD" />);

		expect(markup).toContain('aria-expanded="false"');
		expect(markup).toContain("group-data-[selected=true]:grid");
		expect(markup).toContain("1 · precursor");
		expect(markup).toContain("2 · readiness");
		expect(markup).toContain("war room");
		expect(markup).toContain("causal search");
		expect(markup).toContain("3 · execution + risk");
		expect(markup).toContain('data-df="symbol"');
								expect(markup).toContain('data-df="action"');
		expect(markup).not.toContain("edge=");
	});

	it("renders the live search tree through the chain", () => {
		const markup = renderToStaticMarkup(<DecisionChain symbol="BTC/USD" />);

		// The stage must receive the real telemetry decision, not a stripped
		// projection: passing the thesis object silently drops the trace and
		// renders "no search this round" forever.
		expect(markup).toContain("war-room-consensus");
		expect(markup).toContain("explosive_pump");
		expect(markup).not.toContain("no search this round");
	});

	it("keeps a row addressed by symbol rather than by ring position", () => {
		// A row must resolve its own symbol. Adding another symbol's decision
		// must not change what this row renders — the failure the ring-indexed
		// version had, where a later frame repainted a row with foreign data.
		const other = new DecisionT("d2", "nothing", "ETH/USD");
		other.trace = mockTrace();
		decisionStore.actions.add(other);

		const markup = renderToStaticMarkup(<DecisionChain symbol="BTC/USD" />);

		expect(markup).toContain("BTC/USD");
		expect(markup).not.toContain("ETH/USD");
	});

	it("paints the row from the decision already in hand", () => {
		const markup = renderToStaticMarkup(<DecisionChain symbol="BTC/USD" />);

		// The row's fields are painted imperatively by the store subscription;
		// without seeding they render as an empty outline until the next frame.
		expect(markup).toContain("BTC/USD");
	});
});

import * as flatbuffers from "flatbuffers";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import { equityStore } from "#/collections/app";
import { Balance } from "#/components/balance";
import { EnvelopeState } from "#/providers/telemetry/telemetry/envelope-state";
import { EquityFrame } from "#/providers/telemetry/telemetry/equity-frame";

/*
encodeStateWithEquity builds a real EnvelopeState buffer carrying an equity
frame, the same shape types.Envelope.EncodeBytes produces on the Go side. The
test decodes it back rather than hand-building a stub, so a schema field that
stopped being written would actually fail here.
*/
const encodeStateWithEquity = (
	cash: string,
	unrealized: string,
	equity: string,
): EnvelopeState => {
	const builder = new flatbuffers.Builder(0);

	const cashOffset = builder.createString(cash);
	const unrealizedOffset = builder.createString(unrealized);
	const equityOffset = builder.createString(equity);

	EquityFrame.startEquityFrame(builder);
	EquityFrame.addCash(builder, cashOffset);
	EquityFrame.addUnrealized(builder, unrealizedOffset);
	EquityFrame.addEquity(builder, equityOffset);
	const frame = EquityFrame.endEquityFrame(builder);

	EnvelopeState.startEnvelopeState(builder);
	EnvelopeState.addEquity(builder, frame);
	const state = EnvelopeState.endEnvelopeState(builder);

	builder.finish(state);

	return EnvelopeState.getRootAsEnvelopeState(
		new flatbuffers.ByteBuffer(builder.asUint8Array()),
	);
};

describe("Balance", () => {
	beforeEach(() => {
		equityStore.state.clear();
	});

	it("renders a placeholder before any valuation has arrived", () => {
		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain('data-balance="cash"');
		expect(markup).toContain('data-balance="unrealized"');
		expect(markup).toContain('data-balance="equity"');
		expect(markup).toContain("—");
	});

	it("renders the valuation carried on an envelope state", () => {
		const state = encodeStateWithEquity("1000", "-25.5", "974.5");
		const equity = state.equity(new EquityFrame());

		expect(equity).not.toBeNull();
		equityStore.actions.add(equity as EquityFrame);

		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain("1000.00");
		expect(markup).toContain("-25.50");
		expect(markup).toContain("974.50");
	});

	/*
	The ride is gated on unrealized rather than equity. Equity is above zero the
	moment the wallet is funded, so gating on it would leave the lambo on
	permanently and it would stop meaning anything.
	*/
	it("hides the lambo while the book is down", () => {
		equityStore.actions.add(
			encodeStateWithEquity("1000", "-25.5", "974.5").equity(
				new EquityFrame(),
			) as EquityFrame,
		);

		expect(renderToStaticMarkup(<Balance />)).not.toContain("lambo.png");
	});

	it("rides the lambo behind equity while the book is up", () => {
		equityStore.actions.add(
			encodeStateWithEquity("1000", "25.5", "1025.5").equity(
				new EquityFrame(),
			) as EquityFrame,
		);

		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain("lambo.png");
		/* Decoration only: it never enters the accessibility tree. */
		expect(markup).toContain('aria-hidden="true"');
	});

	it("hides the lambo at exactly flat", () => {
		equityStore.actions.add(
			encodeStateWithEquity("1000", "0", "1000").equity(
				new EquityFrame(),
			) as EquityFrame,
		);

		expect(renderToStaticMarkup(<Balance />)).not.toContain("lambo.png");
	});

	it("keeps the last known valuation when a later frame omits it", () => {
		equityStore.actions.add(
			encodeStateWithEquity("1000", "-25.5", "974.5").equity(
				new EquityFrame(),
			) as EquityFrame,
		);

		// An envelope with an empty equity frame must not blank a balance the
		// dashboard has already been shown.
		equityStore.actions.add(
			encodeStateWithEquity("", "", "").equity(
				new EquityFrame(),
			) as EquityFrame,
		);

		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain("974.50");
	});
});

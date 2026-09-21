import * as flatbuffers from "flatbuffers";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import { cashAtom, equityAtom, unrealizedAtom, updateEquity } from "#/collections/app";
import { Balance } from "#/components/balance";
import { EquityFrame } from "#/providers/telemetry/telemetry/equity-frame";

/*
encodeEquityFrame builds a real EquityFrame buffer, the same shape the wire
produces on the Go side. The test decodes it back rather than hand-building a stub,
so a schema field that stopped being written would actually fail here.
*/
const encodeEquityFrame = (
	cash: string,
	unrealized: string,
	equity: string,
): EquityFrame => {
	const builder = new flatbuffers.Builder(0);

	const cashOffset = builder.createString(cash);
	const unrealizedOffset = builder.createString(unrealized);
	const equityOffset = builder.createString(equity);

	EquityFrame.startEquityFrame(builder);
	EquityFrame.addCash(builder, cashOffset);
	EquityFrame.addUnrealized(builder, unrealizedOffset);
	EquityFrame.addEquity(builder, equityOffset);
	const frame = EquityFrame.endEquityFrame(builder);

	builder.finish(frame);

	return EquityFrame.getRootAsEquityFrame(
		new flatbuffers.ByteBuffer(builder.asUint8Array()),
	);
};

const applyEquityFrame = (frame: EquityFrame) => {
	updateEquity(frame.cash() ?? "", frame.unrealized() ?? "", frame.equity() ?? "");
};

describe("Balance", () => {
	beforeEach(() => {
		cashAtom.set("");
		unrealizedAtom.set("");
		equityAtom.set("");
	});

	it("renders a placeholder before any valuation has arrived", () => {
		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain('data-balance="cash"');
		expect(markup).toContain('data-balance="unrealized"');
		expect(markup).toContain('data-balance="equity"');
		expect(markup).toContain("—");
	});

	it("renders the valuation carried on an equity frame", () => {
		const equity = encodeEquityFrame("1000", "-25.5", "974.5");

		expect(equity).not.toBeNull();
		applyEquityFrame(equity);

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
		applyEquityFrame(
			encodeEquityFrame("1000", "-25.5", "974.5"),
		);

		expect(renderToStaticMarkup(<Balance />)).not.toContain("lambo.png");
	});

	it("rides the lambo behind equity while the book is up", () => {
		applyEquityFrame(
			encodeEquityFrame("1000", "25.5", "1025.5"),
		);

		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain("lambo.png");
		/* Decoration only: it never enters the accessibility tree. */
		expect(markup).toContain('aria-hidden="true"');
	});

	it("hides the lambo at exactly flat", () => {
		applyEquityFrame(
			encodeEquityFrame("1000", "0", "1000"),
		);

		expect(renderToStaticMarkup(<Balance />)).not.toContain("lambo.png");
	});

	it("keeps the last known valuation when a later frame omits it", () => {
		applyEquityFrame(
			encodeEquityFrame("1000", "-25.5", "974.5"),
		);

		// A message with an empty equity frame must not blank a balance the
		// dashboard has already been shown.
		applyEquityFrame(
			encodeEquityFrame("", "", ""),
		);

		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain("974.50");
	});
});

import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import { atoms, RingBuffer, signals } from "#/collections/app";
import { Value } from "#/types/value";
import { TerminalTopBar } from "./terminal-top-bar";

describe("TerminalTopBar", () => {
	beforeEach(() => {
		atoms.tickCount.set(0);
		signals.training.setState(() => ({}));
	});

	it("renders the tick readout", () => {
		const html = renderToStaticMarkup(<TerminalTopBar />);
		expect(html).not.toContain("Observations");
		expect(html).toContain("Ticks");
		expect(html).not.toContain('data-observations="true"');
		expect(html).toContain('data-tick="true"');
	});

	it("renders updated tick count", () => {
		atoms.tickCount.set(45678);
		const html = renderToStaticMarkup(<TerminalTopBar />);
		expect(html).toContain("45,678");
	});

	it("renders training steps as ticks when available", () => {
		const ring = new RingBuffer<Value>(50);
		const val = new Value({ tick: 1234n });
		ring.add(val);

		signals.training.setState(() => ({ "BTC/USD": ring }));

		const html = renderToStaticMarkup(<TerminalTopBar />);
		expect(html).toContain("1,234");
	});
});

// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import { Value } from "#/types/value";
import { Count } from "./count";

/*
lot builds the per-symbol position frame the desk broadcasts: scope is the
symbol's slash escaped to a dash, the "position" layer holds the position
JSON exactly as the Go Append addressed it.
*/
const lot = (symbol: string, virtual = false) => {
	const value = new Value({
		origin: "positions",
		role: "update",
		scope: symbol.replace(/\//g, "-"),
	});

	const payload = new TextEncoder().encode(
		JSON.stringify({
			id: symbol,
			symbol,
			virtual,
			opened_at: "2026-10-18T08:00:00Z",
			closed_at: null,
			qty: "1",
			entry_price: "1",
			mark: "1",
		}),
	);

	value.layers = [payload];
	value.index = [`${value.prefix}/position`];

	return value;
};

const setLots = (lots: Value[]) => {
	signals.positions.setState(() =>
		Object.fromEntries(lots.map((frame) => [frame.scope, frame])),
	);
};

afterEach(() => {
	cleanup();
	bookAtom.set("paper");
	signals.positions.setState(() => ({}));
});

describe("Count", () => {
	it("counts the paper lots the open-positions panel is showing", () => {
		setLots([lot("BTC/USD"), lot("SENT/USD"), lot("APE/USD", true)]);

		const view = render(<Count />);

		expect(view.getByText("2")).toBeTruthy();
	});

	it("counts virtual lots when that book is selected", () => {
		bookAtom.set("virtual");
		setLots([lot("BTC/USD"), lot("APE/USD", true)]);

		const view = render(<Count />);

		expect(view.getByText("1")).toBeTruthy();
	});
});

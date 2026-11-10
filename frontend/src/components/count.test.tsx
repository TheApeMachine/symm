// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { Position } from "#/collections/types";
import { Count } from "./count";

const lot = (
	symbol: string,
	tier_key: "paper" | "virtual" | "real" = "paper",
): Position => ({
	id: symbol,
	symbol,
	tier_key,
	entry_path: "R01",
	exit_path: "",
	opened_at: "2026-10-18T08:00:00Z",
	closed_at: null,
	entry_order: {
		cl_ord_id: `${symbol}-buy`,
		ordertype: "market",
		type: "buy",
		volume: "1",
	},
	fills: [
		{
			side: "buy",
			last_qty: "1",
			last_price: "1",
			avg_price: "1",
			cost: "1",
		},
	],
	mark: "1",
});

const setLots = (lots: Position[]) => {
	signals.positions.setState(() =>
		Object.fromEntries(lots.map((row) => [row.id, row])),
	);
};

afterEach(() => {
	cleanup();
	bookAtom.set("paper");
	signals.positions.setState(() => ({}));
});

describe("Count", () => {
	it("counts the paper lots the open-positions panel is showing", () => {
		bookAtom.set("paper");
		setLots([
			lot("BTC/USD"),
			lot("SENT/USD"),
			lot("APE/USD", "virtual"),
		]);

		const view = render(<Count />);

		expect(view.getByText("2")).toBeTruthy();
	});

	it("counts virtual lots when that book is selected", () => {
		bookAtom.set("virtual");
		setLots([lot("BTC/USD"), lot("APE/USD", "virtual")]);

		const view = render(<Count />);

		expect(view.getByText("1")).toBeTruthy();
	});
});

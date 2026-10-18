// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { deskPositionsAtom } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { DeskPosition } from "#/collections/types";
import { Count } from "./count";

const lot = (symbol: string, virtual = false): DeskPosition => ({
	id: symbol,
	symbol,
	virtual,
	openedAt: "2026-10-18T08:00:00Z",
	closedAt: null,
	qty: "1",
	entryPrice: "1",
	mark: "1",
	entryFee: null,
	exitFee: null,
	exitVwap: null,
	exitRoom: null,
	returnPct: null,
	holdNs: 0,
});

afterEach(() => {
	cleanup();
	bookAtom.set("paper");
	deskPositionsAtom.set([]);
});

describe("Count", () => {
	it("counts the paper lots the open-positions panel is showing", () => {
		deskPositionsAtom.set([
			lot("BTC/USD"),
			lot("SENT/USD"),
			lot("APE/USD", true),
		]);

		const view = render(<Count />);

		expect(view.getByText("2")).toBeTruthy();
	});

	it("counts virtual lots when that book is selected", () => {
		bookAtom.set("virtual");
		deskPositionsAtom.set([lot("BTC/USD"), lot("APE/USD", true)]);

		const view = render(<Count />);

		expect(view.getByText("1")).toBeTruthy();
	});
});

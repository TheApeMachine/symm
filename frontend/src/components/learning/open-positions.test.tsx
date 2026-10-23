// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { deskPositionsAtom } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { DeskPosition } from "#/collections/types";
import { OpenPositions } from "./open-positions";

const lot = (over: Partial<DeskPosition>): DeskPosition => ({
	id: over.id ?? over.symbol ?? "lot",
	symbol: over.symbol ?? "RLC/USD",
	virtual: over.virtual ?? false,
	openedAt: over.openedAt ?? "2026-10-18T08:00:00Z",
	closedAt: over.closedAt ?? null,
	qty: over.qty ?? "2",
	entryPrice: over.entryPrice ?? "1",
	mark: over.mark ?? "1.05",
	entryFee: over.entryFee ?? "0.01",
	exitFee: over.exitFee ?? null,
	exitVwap: over.exitVwap ?? null,
	exitRoom: over.exitRoom ?? null,
	book_return: over.book_return ?? 5,
	returnPct: over.returnPct ?? null,
	holdNs: over.holdNs ?? 75_000_000_000,
});

afterEach(() => {
	cleanup();
	bookAtom.set("paper");
	deskPositionsAtom.set([]);
});

describe("OpenPositions", () => {
	it("shows the empty state when nothing is held", () => {
		const view = render(<OpenPositions />);
		expect(view.getByText("No open positions")).toBeTruthy();
	});

	it("draws a card from the position JSON the desk sent", () => {
		deskPositionsAtom.set([lot({ symbol: "RLC/USD" })]);
		const view = render(<OpenPositions />);
		expect(view.getByText("RLC/USD")).toBeTruthy();
		expect(view.getByText("held 1m 15s")).toBeTruthy();
		expect(view.getByText("5.0%")).toBeTruthy();
		expect(view.getByText("qty 2")).toBeTruthy();
	});

	it("shows exit room on the gauge when the book covers the lot", () => {
		deskPositionsAtom.set([lot({ symbol: "RLC/USD", exitRoom: 12.5 })]);
		const view = render(<OpenPositions />);
		expect(view.getByText("12.5×")).toBeTruthy();
	});

	it("keeps a virtual lot off the paper book", () => {
		deskPositionsAtom.set([
			lot({ symbol: "BTC/USD" }),
			lot({ id: "trial", symbol: "ETH/USD", virtual: true }),
		]);
		const paper = render(<OpenPositions />);
		expect(paper.getByText("BTC/USD")).toBeTruthy();
		expect(paper.queryByText("ETH/USD")).toBeNull();
		paper.unmount();

		bookAtom.set("virtual");
		const virtual = render(<OpenPositions />);
		expect(virtual.getByText("ETH/USD")).toBeTruthy();
		expect(virtual.queryByText("BTC/USD")).toBeNull();
	});
});

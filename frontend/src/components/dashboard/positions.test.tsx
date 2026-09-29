import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { positionsAtom } from "#/collections/app";
import { HoldingT } from "#/providers/telemetry/telemetry/holding";
import { PositionT } from "#/providers/telemetry/telemetry/position";
import { Positions } from "./positions";

describe("Positions", () => {
	it("renders without error when positionsAtom is empty", () => {
		positionsAtom.set([]);
		const markup = renderToStaticMarkup(<Positions />);
		expect(markup).toContain("no open positions");
	});

	it("renders active open positions when positionsAtom contains positions", () => {
		const original = positionsAtom.get();
		try {
			const pos = new PositionT();
			pos.status = "active";
			const holding = new HoldingT();
			holding.symbol = "NMR/USD";
			holding.status = "active";
			holding.pnl = "0.4200";
			holding.entryPrice = "12.345600";
			holding.mark = "12.567800";
			holding.returnPct = 3.41;
			pos.holding = holding;

			positionsAtom.set([pos]);
			const markup = renderToStaticMarkup(<Positions />);
			expect(markup).toContain("NMR/USD");
			expect(markup).toContain("active");
			expect(markup).toContain("0.4200 USD");
			expect(markup).toContain("EXIT");
			expect(markup).toContain("12.345600");
			expect(markup).toContain("12.567800");
			expect(markup).not.toContain("no open positions");
		} finally {
			positionsAtom.set(original);
		}
	});
});

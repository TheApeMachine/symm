import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { positionStore } from "#/collections/app";
import { Positions } from "./positions";

describe("Positions", () => {
	it("renders without error when positionStore is in default fallback state", () => {
		const markup = renderToStaticMarkup(<Positions />);
		expect(markup).toContain("no open positions");
	});

	it("renders without error when positionStore state does not have findLast function", () => {
		const originalState = positionStore.state;
		try {
			positionStore.setState({} as any);
			const markup = renderToStaticMarkup(<Positions />);
			expect(markup).toContain("no open positions");
		} finally {
			positionStore.setState(originalState);
		}
	});

	it("renders active open positions when positionStore contains positions", () => {
		const originalState = positionStore.state;
		try {
			const mockHolding = {
				symbol: () => "NMR/USD",
				status: () => "active",
				pnl: () => "0.4200",
				entryPrice: () => "12.345600",
				mark: () => "12.567800",
				returnPct: () => 3.41,
			};
			const mockPosition = {
				status: () => "active",
				holding: () => mockHolding,
			};
			const mockFrame = {
				rowsLength: () => 1,
				rows: () => mockPosition,
			};

			positionStore.setState(mockFrame as any);
			const markup = renderToStaticMarkup(<Positions />);
			expect(markup).toContain("NMR/USD");
			expect(markup).toContain("active");
			expect(markup).toContain("0.4200 USD");
			expect(markup).toContain("EXIT");
			expect(markup).toContain("12.345600");
			expect(markup).toContain("12.567800");
			expect(markup).not.toContain("no open positions");
		} finally {
			positionStore.setState(originalState);
		}
	});
});

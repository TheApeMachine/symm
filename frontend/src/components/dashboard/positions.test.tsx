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
});

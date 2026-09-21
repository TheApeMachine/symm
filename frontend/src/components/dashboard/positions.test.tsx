import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { signals } from "#/collections/app";
import { Positions } from "./positions";

describe("Positions", () => {
	it("renders without error when signals.position is in default fallback state", () => {
		const markup = renderToStaticMarkup(<Positions />);
		expect(markup).toContain("no open positions");
	});

	it("renders without error when signals.position state does not have findLast function", () => {
		const originalState = signals.position.state;
		try {
			signals.position.setState({} as any);
			const markup = renderToStaticMarkup(<Positions />);
			expect(markup).toContain("no open positions");
		} finally {
			signals.position.setState(() => originalState);
		}
	});
});

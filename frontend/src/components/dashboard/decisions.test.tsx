import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { Decisions } from "./decisions";

vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => () => undefined,
}));

describe("Decisions", () => {
	it("renders one card per real decision, with no pre-allocated slots", () => {
		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup).not.toContain("data-decision-id");
		expect(markup).toContain("waiting for backend decision frames");
		expect(markup).not.toMatch(
			/data-decision-card="true"[^>]*data-decision-card="true"/,
		);
	});
});

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { Decisions } from "./decisions";

vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => () => undefined,
}));

describe("Decisions", () => {
	it("renders each reserved decision slot as a compact reason card", () => {
		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup.match(/data-decision-card="true"/g)).toHaveLength(6);
		expect(markup.match(/data-paint="reason"/g)).toHaveLength(6);
		expect(markup).toContain("line-clamp-2");
		expect(markup).toContain(
			'data-paint-empty="No rejection reason published"',
		);
		expect(markup).toContain('data-paint="symbol"');
		expect(markup).toContain('data-paint="thesisScore"');
		expect(markup).toContain('data-paint="action"');
	});
});

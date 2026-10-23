import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import { atoms } from "#/collections/app";
import { Balance } from "#/components/balance";

describe("Balance", () => {
	beforeEach(() => {
		atoms.cash.set("");
		atoms.unrealized.set("");
		atoms.equity.set("");
	});

	const setEquity = (cash: string, unrealized: string, equity: string) => {
		atoms.cash.set(cash);
		atoms.unrealized.set(unrealized);
		atoms.equity.set(equity);
	};

	it("renders a placeholder before any valuation has arrived", () => {
		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain('data-balance="cash"');
		expect(markup).toContain('data-balance="unrealized"');
		expect(markup).toContain('data-balance="equity"');
		expect(markup).toContain("—");
	});

	it("renders the valuation carried on an equity update", () => {
		setEquity("1000", "-25.5", "974.5");

		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain("1000.00");
		expect(markup).toContain("-25.50");
		expect(markup).toContain("974.50");
	});

	it("hides the lambo while the book is down", () => {
		setEquity("1000", "-25.5", "974.5");

		expect(renderToStaticMarkup(<Balance />)).not.toContain("lambo.png");
	});

	it("rides the lambo behind equity while the book is up", () => {
		setEquity("1000", "25.5", "1025.5");

		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain("lambo.png");
		expect(markup).toContain('aria-hidden="true"');
	});

	it("hides the lambo at exactly flat", () => {
		setEquity("1000", "0", "1000");

		expect(renderToStaticMarkup(<Balance />)).not.toContain("lambo.png");
	});

	it("keeps the last known valuation when a later update omits it", () => {
		setEquity("1000", "-25.5", "974.5");
		setEquity("", "", "");

		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain("974.50");
	});
});

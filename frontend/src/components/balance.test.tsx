import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import { atoms } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import { confidenceAtom } from "#/collections/confidence";
import type { Balance as BalanceReading } from "#/collections/types";
import { Balance } from "#/components/balance";

const reading = (
	cash: number,
	unrealized: number,
	equity: number,
): BalanceReading => ({ cash, unrealized, equity });

describe("Balance", () => {
	beforeEach(() => {
		bookAtom.set("paper");
		atoms.balance.set(null);
		atoms.balances.set({});
		confidenceAtom.set({
			...confidenceAtom.get(),
			real_ceiling: false,
		});
	});

	it("renders a placeholder before any valuation has arrived", () => {
		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain('data-balance="cash"');
		expect(markup).toContain('data-balance="unrealized"');
		expect(markup).toContain('data-balance="equity"');
		expect(markup).toContain("Paper cash");
		expect(markup).toContain("—");
	});

	it("renders the selected ledger's wallet", () => {
		atoms.balances.set({
			virtual: reading(50, 0, 50),
			paper: reading(1000, -25.5, 974.5),
			real: reading(200, 0, 200),
		});

		bookAtom.set("paper");
		expect(renderToStaticMarkup(<Balance />)).toContain("1000.00");
		expect(renderToStaticMarkup(<Balance />)).toContain("Paper cash");

		bookAtom.set("virtual");
		const virtual = renderToStaticMarkup(<Balance />);
		expect(virtual).toContain("50.00");
		expect(virtual).toContain("Virtual cash");
		expect(virtual).not.toContain("1000.00");

		bookAtom.set("real");
		// Hub already pushed balances.real — show it even before confidence.
		const primed = renderToStaticMarkup(<Balance />);
		expect(primed).toContain("200.00");
		expect(primed).toContain("Live cash");
		expect(primed).not.toContain("1000.00");

		atoms.balances.set({
			virtual: reading(50, 0, 50),
			paper: reading(1000, -25.5, 974.5),
		});
		const offline = renderToStaticMarkup(<Balance />);
		expect(offline).toContain("Live cash (off)");
		expect(offline).toContain("—");
		expect(offline).not.toContain("200.00");
		expect(offline).not.toContain("1000.00");

		atoms.balances.set({
			virtual: reading(50, 0, 50),
			paper: reading(1000, -25.5, 974.5),
			real: reading(200, 0, 200),
		});
		confidenceAtom.set({
			...confidenceAtom.get(),
			real_ceiling: true,
		});
		const live = renderToStaticMarkup(<Balance />);
		expect(live).toContain("200.00");
		expect(live).toContain("Live cash");
		expect(live).not.toContain("1000.00");
	});

	it("hides the lambo while the book is down", () => {
		atoms.balances.set({ paper: reading(1000, -25.5, 974.5) });

		expect(renderToStaticMarkup(<Balance />)).not.toContain("lambo.png");
	});

	it("rides the lambo behind equity while the book is up", () => {
		atoms.balances.set({ paper: reading(1000, 25.5, 1025.5) });

		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain("lambo.png");
		expect(markup).toContain('aria-hidden="true"');
	});

	it("hides the lambo at exactly flat", () => {
		atoms.balances.set({ paper: reading(1000, 0, 1000) });

		expect(renderToStaticMarkup(<Balance />)).not.toContain("lambo.png");
	});
});

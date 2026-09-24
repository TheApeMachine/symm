import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import { boundAtom } from "#/collections/app";
import { Balance } from "#/components/balance";

/* shell binds values the way the running graph delivers them to ui_shell. */
const shell = (values: Record<string, number>) =>
	boundAtom.set({
		ui_shell: Object.fromEntries(
			Object.entries(values).map(([component, value]) => [
				component,
				{ value },
			]),
		),
	});

describe("Balance", () => {
	beforeEach(() => {
		boundAtom.set({});
	});

	it("renders a placeholder before the graph has reported the wallet", () => {
		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain('data-balance="cash"');
		expect(markup).toContain('data-balance="pnl"');
		expect(markup).toContain("—");
	});

	it("renders the wallet the graph bound", () => {
		shell({ cash: 974.5, pnl: -25.5 });

		const markup = renderToStaticMarkup(<Balance />);

		expect(markup).toContain("974.50");
		expect(markup).toContain("-25.50");
	});

	/*
	The ride is gated on P&L rather than cash. Cash is above zero the moment the
	wallet is funded, so gating on it would leave the lambo on permanently and it
	would stop meaning anything.
	*/
	it("rides the lambo only while the wallet is in profit", () => {
		shell({ cash: 974.5, pnl: -25.5 });
		expect(renderToStaticMarkup(<Balance />)).not.toContain("lambo.png");

		shell({ cash: 1010, pnl: 10 });
		expect(renderToStaticMarkup(<Balance />)).toContain("lambo.png");
	});
});

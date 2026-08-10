import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DEFAULT_FOCUS_SYMBOL } from "#/collections/app";
import { Engine } from "#/components/engine";

describe("Engine", () => {
	it("scopes readiness gates to the focused symbol", () => {
		const markup = renderToStaticMarkup(<Engine />);

		expect(markup).toContain('data-scope="symbol"');
		expect(markup).toContain(`data-filter="${DEFAULT_FOCUS_SYMBOL}"`);
	});
});

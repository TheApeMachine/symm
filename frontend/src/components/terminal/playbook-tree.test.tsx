import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { PlaybookBranchTree } from "#/components/terminal/playbook-tree";

describe("PlaybookBranchTree", () => {
	it("renders empty state when no branches exist", () => {
		const html = renderToStaticMarkup(<PlaybookBranchTree symbol="BTC/USD" />);

		expect(html).toContain("No playbook branches loaded");
	});
});

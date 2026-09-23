import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { KnowledgePanel } from "./knowledge-panel";

describe("KnowledgePanel", () => {
	it("renders learned context evidence section", () => {
		const html = renderToStaticMarkup(<KnowledgePanel />);
		expect(html).toContain("Learned context evidence");
	});
});

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Rail } from "./rail";

describe("Rail component", () => {
	it("renders header, body, and footer with positioning", () => {
		const markup = renderToStaticMarkup(
			<Rail position="left" width="narrow" surface="surface">
				<Rail.Header title="Kernels" meta="5 total" />
				<Rail.Body padding="m">
					<div>Content</div>
				</Rail.Body>
				<Rail.Footer>Status</Rail.Footer>
			</Rail>,
		);

		expect(markup).toContain("Kernels");
		expect(markup).toContain("5 total");
		expect(markup).toContain("Content");
		expect(markup).toContain("Status");
		expect(markup).toContain("border-r");
		expect(markup).toContain("w-[230px]");
	});
});

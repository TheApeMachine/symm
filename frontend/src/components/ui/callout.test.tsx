import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Callout } from "./callout";

describe("Callout component", () => {
	it("renders title, meta, and children with tone styling", () => {
		const markup = renderToStaticMarkup(
			<Callout
				tone="accent"
				title="Why SYMM entered"
				meta={<div>FROZEN AT ENTRY</div>}
			>
				<p>The expected move cleared the cost boundary.</p>
			</Callout>,
		);

		expect(markup).toContain("Why SYMM entered");
		expect(markup).toContain("FROZEN AT ENTRY");
		expect(markup).toContain("The expected move cleared the cost boundary.");
		expect(markup).toContain("[--callout-tone:var(--acc)]");
	});

	it("supports custom description slot", () => {
		const markup = renderToStaticMarkup(
			<Callout tone="neutral" size="s">
				<Callout.Title>Live now</Callout.Title>
				<Callout.Description>
					These values change with the market.
				</Callout.Description>
			</Callout>,
		);

		expect(markup).toContain("Live now");
		expect(markup).toContain("These values change with the market.");
		expect(markup).toContain("rounded-[3px]");
	});
});

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DataRow } from "./data-row";

describe("DataRow component", () => {
	it("renders inline key-value pairs with semantic tones", () => {
		const markup = renderToStaticMarkup(
			<DataRow
				label="regime class"
				value="CLASSIFIED"
				paintKey="winner"
				tone="accent"
			/>,
		);

		expect(markup).toContain("regime class");
		expect(markup).toContain("CLASSIFIED");
		expect(markup).toContain('data-f="winner"');
		expect(markup).toContain("text-(--acc)");
	});

	it("renders detailed layout with help explanation", () => {
		const markup = renderToStaticMarkup(
			<DataRow
				label="entry price"
				value="0.00049"
				help="Volume weighted price at entry"
				tone="f1"
			/>,
		);

		expect(markup).toContain("entry price");
		expect(markup).toContain("0.00049");
		expect(markup).toContain("Volume weighted price at entry");
	});

	it("DataRow.Group renders a column of rows with hairlines", () => {
		const markup = renderToStaticMarkup(
			<DataRow.Group>
				<DataRow label="One" value="1" />
				<DataRow label="Two" value="2" />
			</DataRow.Group>,
		);

		expect(markup).toContain("One");
		expect(markup).toContain("Two");
	});
});

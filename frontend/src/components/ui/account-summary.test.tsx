import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AccountSummary } from "./account-summary";

describe("AccountSummary", () => {
	it("preserves decimal money and UInt64 counters from the native account", () => {
		const html = renderToStaticMarkup(<AccountSummary cash="199.1234567890123456789" pnl="-0.0000000000000000001" observations="9007199254740993" outcomes="0" phase="paper" meanEdge={0} edgeDefined={false} />);
		expect(html).toContain("199.1234567890123456789");
		expect(html).toContain("-0.0000000000000000001");
		expect(html).toContain("9007199254740993");
		expect(html).not.toContain("0.0%");
	});
	it("shows measured negative edge only when the account defines it", () => {
		const html = renderToStaticMarkup(<AccountSummary meanEdge={-0.012} edgeDefined standardError={0.002} uncertaintyDefined phase="paper" />);
		expect(html).toContain("-1.2%");
		expect(html).toContain("0.2%");
	});
});

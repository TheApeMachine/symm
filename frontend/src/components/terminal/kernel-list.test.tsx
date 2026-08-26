import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DEFAULT_KERNELS } from "#/collections/app";
import { KernelList } from "./kernel-list";

describe("KernelList", () => {
	it("binds every kernel row to its source and renders the mockup row anatomy", () => {
		const markup = renderToStaticMarkup(
			<KernelList sources={DEFAULT_KERNELS} />,
		);
		const rows = [...markup.matchAll(/data-kernel="([^"]+)"/g)].map(
			(m) => m[1],
		);

		expect(rows).toHaveLength(DEFAULT_KERNELS.length);
		expect(new Set(rows)).toEqual(new Set(DEFAULT_KERNELS));
		// Each row names the kernel, carries a status badge, a sub line, a
		// sparkline with area fill, and a confidence bar with a reading.
		expect(markup).toContain("Hawkes process");
		expect(markup).toContain("branching η");
		expect(markup).toContain("Standby");
		expect(markup).toContain('data-k="snr1"');
		expect(markup).toContain('data-k="conf"');
		expect(markup).toMatch(/<polyline/);
	});
});

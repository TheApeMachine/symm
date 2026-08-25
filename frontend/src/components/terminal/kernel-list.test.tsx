import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DEFAULT_KERNELS } from "#/collections/app";
import { KernelList } from "./kernel-list";

describe("KernelList", () => {
	it("binds every kernel row to its source and the shared snr reading", () => {
		const markup = renderToStaticMarkup(<KernelList sources={DEFAULT_KERNELS} />);
		const rows = [...markup.matchAll(/data-kernel="([^"]+)"/g)].map((m) => m[1]);

		expect(rows).toHaveLength(DEFAULT_KERNELS.length);
		expect(new Set(rows)).toEqual(new Set(DEFAULT_KERNELS));
		expect(markup).toContain('data-k="snr1"');
	});
});

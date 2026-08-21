import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DEFAULT_KERNELS } from "#/collections/app";
import { KernelList } from "./kernel-list";

const SEPARATION_RAW = "metrics.hypothesis_separation.raw";

describe("KernelList", () => {
	it("binds every kernel trace to the shared hypothesis-separation metric", () => {
		const markup = renderToStaticMarkup(
			<KernelList sources={DEFAULT_KERNELS} />,
		);
		const traceBindings = [...markup.matchAll(/data-append="([^"]+)"/g)].map(
			(match) => match[1],
		);

		expect(traceBindings).toHaveLength(DEFAULT_KERNELS.length * 2);
		expect(new Set(traceBindings)).toEqual(new Set([SEPARATION_RAW]));

		/*
			Every row paints the same hypothesis-separation reading; source scope
			selects which measurement, not which metric.
		*/
		expect(markup).toContain(`data-paint="${SEPARATION_RAW}"`);
	});
});

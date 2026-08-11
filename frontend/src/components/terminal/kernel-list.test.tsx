import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DEFAULT_KERNELS } from "#/collections/app";
import { KernelList } from "./kernel-list";

describe("KernelList", () => {
	it("binds every full-size kernel trace and value to SNR", () => {
		const markup = renderToStaticMarkup(
			<KernelList sources={DEFAULT_KERNELS} />,
		);
		const traceBindings = [...markup.matchAll(/data-append="([^"]+)"/g)].map(
			(match) => match[1],
		);

		expect(traceBindings).toHaveLength(DEFAULT_KERNELS.length * 2);
		expect(new Set(traceBindings)).toEqual(new Set(["metrics.snr.normalized"]));
		expect(markup.match(/data-paint="metrics\.snr\.raw"/g)).toHaveLength(
			DEFAULT_KERNELS.length,
		);
	});
});

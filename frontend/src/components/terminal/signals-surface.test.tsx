import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";
import { appStore, DEFAULT_KERNELS } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { SignalsSurface, signalsSurfaceSources } from "./signals-surface";

const emptyState = () => ({
	measurements: {},
	symbols: {},
	sources: new Set<string>(),
	tick: 0,
});

describe("SignalsSurface", () => {
	afterEach(() => {
		appStore.setState((state) => ({
			...state,
			kernels: DEFAULT_KERNELS,
		}));
		measurementsStore.setState(() => emptyState());
		terminalStore.setState((state) => ({
			...state,
			selectedSource: "fluid",
			focusSymbol: "stream",
		}));
	});

	it("renders kernels from measurement sources", () => {
		appStore.setState((state) => ({
			...state,
			kernels: ["fluid", "prediction", "cognitive"],
		}));

		const html = renderToString(<SignalsSurface />);

		expect(html).toContain("Fluid dynamics");
		expect(html).toContain("Predictive coding");
		expect(html).toContain("Cognitive memory");
		expect(html).toContain("Hawkes process");
		expect(html).toContain("Causal ladder");
		expect(html).not.toContain(">measurements<");
		expect(html).not.toContain(">symbols<");
	});

	it("keeps configured presentation kernels with backend sources", () => {
		const sources = signalsSurfaceSources(
			["prediction", "cognitive"],
			emptyState(),
		);

		expect(sources).toContain("prediction");
		expect(sources).toContain("cognitive");
		expect(sources).toContain("hawkes");
	});
});

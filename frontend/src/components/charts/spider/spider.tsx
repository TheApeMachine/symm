import { type RefObject, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import {
	drawSignalSpider,
	type SpiderControls,
} from "#/components/charts/spider/init";

export type SpiderBridge = {
	set: (source: string, value: number) => void;
	ready: boolean;
};

const SCROLL_MS = 250;

/*
SpiderChart shows the live signal landscape as a radar: one petal per signal,
radius is its confidence (0-100), refreshed on a fixed cadence.
*/
export const SpiderChart = ({
	sources,
	labels,
	bridgeRef,
}: {
	sources: string[];
	labels: Record<string, string>;
	bridgeRef: RefObject<SpiderBridge>;
}) => {
	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) =>
			drawSignalSpider(
				rootElement,
				sources.map((source) => labels[source] ?? source),
			),
		[sources, labels],
	);

	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawSignalSpider>) => {
			const current = new Map<string, number>(sources.map((s) => [s, 0]));
			const bridge = bridgeRef.current;

			bridge.set = (source, value) => current.set(source, value);
			bridge.ready = true;

			const controls = result.controls as SpiderControls;
			const interval = setInterval(() => {
				controls.update(sources.map((s) => (current.get(s) ?? 0) * 100));
			}, SCROLL_MS);

			return () => {
				clearInterval(interval);
				bridge.set = () => {};
				bridge.ready = false;
			};
		},
		[sources, bridgeRef],
	);

	return (
		<SciChartReact
			key={sources.join(",")}
			initChart={initChart}
			onInit={onInit}
			className="h-full w-full"
			style={{ width: "100%", height: "100%" }}
		/>
	);
};

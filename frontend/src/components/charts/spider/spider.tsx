import { type RefObject, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import {
	drawSignalSpider,
	type SpiderControls,
} from "#/components/charts/spider/init";
import {
	attachSpiderBridge,
	detachSpiderBridge,
	type SpiderBridge,
	scaleSpiderRadarValues,
} from "#/components/charts/spider/spider-bridge";

export type { SpiderBridge } from "#/components/charts/spider/spider-bridge";

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
			const bridge = bridgeRef.current;
			const controls = result.controls as SpiderControls;

			const applyValues = (values: Record<string, number>) => {
				controls.update(scaleSpiderRadarValues(sources, values));
			};

			attachSpiderBridge(bridge, applyValues);

			return () => {
				detachSpiderBridge(bridge);
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

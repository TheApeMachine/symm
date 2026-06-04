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
			const current = new Map<string, number>(
				sources.map((source) => [source, 0]),
			);
			const bridge = bridgeRef.current;
			const controls = result.controls as SpiderControls;

			bridge.set = (source, value) => {
				current.set(source, value);
				controls.update(sources.map((axis) => (current.get(axis) ?? 0) * 100));
			};
			bridge.ready = true;

			return () => {
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

import { memo, type RefObject, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import { drawExample } from "#/components/charts/prediction/init-predictions-chart";

export type PredictionBridge = {
	append: (tsSec: number, confidence: number) => void;
	ready: boolean;
};

/*
PredictionChart plots the live "prediction" signal's confidence as a time series.
The websocket feeds samples through the bridge; the chart owns its scrolling axis.
*/
export const PredictionChart = memo(function PredictionChart({
	bridgeRef,
}: {
	bridgeRef: RefObject<PredictionBridge>;
}) {
	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawExample>) => {
			const bridge = bridgeRef.current;
			const { appendReading } = result.controls;

			bridge.append = (tsSec, confidence) =>
				appendReading({ kind: "average", x: tsSec, value: confidence });
			bridge.ready = true;

			return () => {
				bridge.append = () => {};
				bridge.ready = false;
			};
		},
		[bridgeRef],
	);

	return (
		<SciChartReact
			className="h-full w-full"
			style={{ width: "100%", height: "100%" }}
			initChart={drawExample}
			onInit={onInit}
		/>
	);
});

import { memo, type RefObject, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import { drawExample } from "#/components/charts/prediction/init-predictions-chart";

export type PredictionBridge = {
	append: (tsSec: number, confidence: number) => void;
	ready: boolean;
	pending: [number, number][];
};

export const PredictionChart = memo(function PredictionChart({
	bridgeRef,
}: {
	bridgeRef: RefObject<PredictionBridge>;
}) {
	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawExample>) => {
			const bridge = bridgeRef.current;
			const { appendReading } = result.controls;

			bridge.append = (tsSec, confidence) => {
				appendReading({ kind: "average", x: tsSec, value: confidence });
			};
			bridge.ready = true;

			for (const [tsSec, confidence] of bridge.pending) {
				bridge.append(tsSec, confidence);
			}

			bridge.pending = [];

			return () => {
				bridge.append = () => {};
				bridge.ready = false;
				bridge.pending = [];
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

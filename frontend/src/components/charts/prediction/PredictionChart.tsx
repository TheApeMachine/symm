import { memo, type RefObject, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import { drawExample } from "#/components/charts/prediction/init-predictions-chart";
import type { PredictionReading } from "#/components/charts/prediction/predictions-data-provider";

export type PredictionBridge = {
	append: (reading: PredictionReading) => void;
	ready: boolean;
	pending: PredictionReading[];
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

			bridge.append = appendReading;
			bridge.ready = true;

			for (const reading of bridge.pending) {
				bridge.append(reading);
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

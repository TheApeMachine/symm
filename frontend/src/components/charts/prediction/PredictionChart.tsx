import { memo, useCallback } from "react";
import { SciChartReact } from "scichart-react";

import {
	initPredictionChart,
	type TPredictionChartInitResult,
} from "#/components/charts/prediction/init-prediction-chart";
import { registerPredictionChart } from "#/components/charts/prediction/prediction-chart-wire";

export const PredictionChart = memo(function PredictionChart() {
	const initChart = useCallback((rootElement: string | HTMLDivElement) => {
		if (typeof rootElement === "string") {
			throw new Error("initPredictionChart requires an HTMLDivElement root");
		}

		return initPredictionChart(rootElement);
	}, []);

	const onInit = useCallback(
		(result: TPredictionChartInitResult) =>
			registerPredictionChart(result.appendReading),
		[],
	);

	return (
		<SciChartReact
			className="h-full w-full"
			style={{ width: "100%", height: "100%" }}
			initChart={initChart}
			onInit={onInit}
		/>
	);
});

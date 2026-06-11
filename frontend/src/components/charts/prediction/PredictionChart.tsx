import { memo } from "react";
import { SciChartReact } from "scichart-react";
import { appStore } from "#/collections/app";
import { initPredictionChart } from "#/components/charts/prediction/init-prediction-chart";

export const PredictionChart = memo(function PredictionChart() {
	return (
		<SciChartReact
			className="h-full w-full"
			style={{ width: "100%", height: "100%" }}
			initChart={(rootElement) => {
				if (typeof rootElement === "string") {
					throw new Error(
						"initPredictionChart requires an HTMLDivElement root",
					);
				}

				return initPredictionChart(rootElement);
			}}
			onInit={(result) => {
				appStore.actions.updatePredictionUpdater(result.addData);

				return () => appStore.actions.updatePredictionUpdater(null);
			}}
		/>
	);
});

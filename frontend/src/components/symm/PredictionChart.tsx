import { SciChartReact } from "scichart-react";
import { drawExample } from "#/components/symm/init-predictions-chart";
import { memo } from "react";
import "#/lib/symm/scichart-setup";

export const PredictionChart = memo(function PredictionChart() {
	return (
		<SciChartReact
			className="h-full w-full"
			initChart={drawExample}
		></SciChartReact>
	);
});

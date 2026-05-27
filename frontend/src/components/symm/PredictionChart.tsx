import { memo, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import { drawExample } from "#/components/symm/init-predictions-chart";
import { PredictionsDataProvider } from "#/components/symm/predictions-data-provider";
import "#/lib/symm/scichart-setup";

export const PredictionChart = memo(function PredictionChart() {
	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawExample>) =>
			PredictionsDataProvider.registerSink((reading) => {
				result.controls.appendReading(reading);
			}),
		[],
	);

	return (
		<SciChartReact
			className="h-full w-full"
			initChart={drawExample}
			onInit={onInit}
		/>
	);
});

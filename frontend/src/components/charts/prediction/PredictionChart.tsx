import { memo, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import { drawExample } from "#/components/charts/prediction/init-predictions-chart";
import { useSymmTelemetryStores } from "#/lib/symm/telemetry-context";
import "#/lib/symm/scichart-setup";

export const PredictionChart = memo(function PredictionChart() {
	const stores = useSymmTelemetryStores();

	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawExample>) =>
			stores.predictions.registerSink((reading) => {
				result.controls.appendReading(reading);
			}),
		[stores.predictions],
	);

	return (
		<SciChartReact
			className="h-full w-full"
			initChart={drawExample}
			onInit={onInit}
		/>
	);
});

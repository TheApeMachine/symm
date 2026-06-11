import { memo } from "react";
import { SciChartReact } from "scichart-react";
import { appStore } from "#/collections/app";
import { initFluidSurfaceChart } from "#/components/charts/fluid/init-fluid-surface-chart";

export const FluidFieldSurfaceChart = memo(function FluidFieldSurfaceChart() {
	return (
		<SciChartReact
			style={{ height: "100%", width: "100%" }}
			initChart={initFluidSurfaceChart}
			onInit={(result) => {
				appStore.actions.updateFluidUpdater(result.addData);

				return () => appStore.actions.updateFluidUpdater(null);
			}}
		/>
	);
});

import { memo } from "react";
import { SciChartReact } from "scichart-react";
import { appStore } from "#/collections/app";
import { initManifoldSurfaceChart } from "#/components/charts/manifold/init-manifold-surface-chart";

export const ManifoldSurfaceChart = memo(function ManifoldSurfaceChart() {
	return (
		<SciChartReact
			style={{ height: "100%", width: "100%" }}
			initChart={initManifoldSurfaceChart}
			onInit={(result) => {
				appStore.actions.updateManifoldUpdater(result.addData);

				return () => appStore.actions.updateManifoldUpdater(null);
			}}
		/>
	);
});

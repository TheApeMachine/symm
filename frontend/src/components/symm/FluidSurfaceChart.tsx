import { memo, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import type { SciChart3DSurface } from "scichart";

import { FluidDataProvider } from "#/components/symm/fluid-data-provider";
import { drawExample } from "./init-fluid-surface-chart";
import "#/lib/symm/scichart-setup";

/** Live 3D surface over change% × vol bins. */
export const FluidSurfaceChart = memo(function FluidSurfaceChart() {
	const onInit = useCallback(
		(initResult: TResolvedReturnType<typeof drawExample>) => {
			const unregister = FluidDataProvider.registerSink(
				initResult.controls.update,
			);

			return () => {
				unregister();
				initResult.controls.dispose();
			};
		},
		[],
	);

	return (
		<SciChartReact<SciChart3DSurface, TResolvedReturnType<typeof drawExample>>
			initChart={drawExample}
			onInit={onInit}
			className="h-full w-full"
		/>
	);
});

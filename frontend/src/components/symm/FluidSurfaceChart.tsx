import { memo } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import { drawExample } from "./init-fluid-surface-chart";
import type { SciChart3DSurface } from "scichart";
import "#/lib/symm/scichart-setup";

/** Live 3D terrain of clipped fluid activity over change% × vol bins. */
export const FluidSurfaceChart = memo(function FluidSurfaceChart() {
	return (
		<SciChartReact<SciChart3DSurface, TResolvedReturnType<typeof drawExample>>
			initChart={drawExample}
			onInit={(initResult: TResolvedReturnType<typeof drawExample>) => {
				const { controls } = initResult;
				controls.startUpdate();

				// Return a cleanup function
				return () => {
					controls.stopUpdate();
				};
			}}
			className="h-full w-full"
		/>
	);
});

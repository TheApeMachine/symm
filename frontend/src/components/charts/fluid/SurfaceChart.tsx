import { type RefObject, useCallback } from "react";
import type { SciChart3DSurface } from "scichart";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import {
	attachFluidPush,
	detachFluidPush,
	type FluidPushBridge,
} from "#/components/charts/fluid/fluid-push-bridge";
import { initFluidSurfaceChart } from "#/components/charts/fluid/init-fluid-surface-chart";

export const FluidFieldSurfaceChart = ({
	bridgeRef,
}: {
	bridgeRef: RefObject<FluidPushBridge>;
}) => {
	const onInit = useCallback(
		(result: TResolvedReturnType<typeof initFluidSurfaceChart>) => {
			const bridge = bridgeRef.current;

			if (!bridge) {
				return;
			}

			attachFluidPush(bridge, result.controls.push);

			return () => {
				detachFluidPush(bridge);
			};
		},
		[bridgeRef],
	);

	return (
		<SciChartReact<
			SciChart3DSurface,
			TResolvedReturnType<typeof initFluidSurfaceChart>
		>
			initChart={initFluidSurfaceChart}
			onInit={onInit}
			style={{ height: "100%", width: "100%" }}
		/>
	);
};

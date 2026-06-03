import { type MutableRefObject, useCallback } from "react";
import type { SciChart3DSurface } from "scichart";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import { initFluidSurfaceChart } from "#/components/charts/fluid/init-fluid-surface-chart";
import type { FluidPushBridge } from "#/routes/index";

export const FluidFieldSurfaceChart = ({
	bridgeRef,
}: {
	bridgeRef: MutableRefObject<FluidPushBridge>;
}) => {
	const onInit = useCallback(
		(result: TResolvedReturnType<typeof initFluidSurfaceChart>) => {
			const bridge = bridgeRef.current;
			bridge.push = result.controls.push;
			bridge.ready = true;

			for (const frame of bridge.pending) {
				bridge.push(frame);
			}

			bridge.pending = [];

			return () => {
				bridge.push = () => {};
				bridge.ready = false;
				bridge.pending = [];
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
			style={{ height: "100%", width: "50%" }}
		/>
	);
};

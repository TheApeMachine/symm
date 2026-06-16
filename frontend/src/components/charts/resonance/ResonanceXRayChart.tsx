import { memo, useRef } from "react";
import { SciChartReact } from "scichart-react";
import { appStore } from "#/collections/app";
import { initResonanceLatentChart } from "#/components/charts/resonance/init-resonance-latent-chart";
import { initResonanceXRayChart } from "#/components/charts/resonance/init-resonance-xray-chart";
import { Flex } from "#/components/ui/flex";

type ResonanceFrameUpdater = (frame: Record<string, unknown>) => void;

export const ResonanceXRayChart = memo(function ResonanceXRayChart() {
	const updatersRef = useRef<{
		xray: ResonanceFrameUpdater | null;
		latent: ResonanceFrameUpdater | null;
	}>({
		xray: null,
		latent: null,
	});

	const registerResonanceFanOut = () => {
		appStore.actions.updateResonanceUpdater((frame) => {
			updatersRef.current.xray?.(frame);
			updatersRef.current.latent?.(frame);
		});
	};

	return (
		<Flex.Row className="h-full w-full min-h-0" gap={1} fullWidth fullHeight>
			<div className="h-full min-h-0 min-w-0 flex-3">
				<SciChartReact
					style={{ height: "100%", width: "100%" }}
					initChart={initResonanceXRayChart}
					onInit={(result) => {
						updatersRef.current.xray = result.addData;
						registerResonanceFanOut();

						return () => {
							updatersRef.current.xray = null;
							registerResonanceFanOut();
						};
					}}
				/>
			</div>
			<div className="h-full min-h-0 min-w-0 flex-2">
				<SciChartReact
					style={{ height: "100%", width: "100%" }}
					initChart={initResonanceLatentChart}
					onInit={(result) => {
						updatersRef.current.latent = result.addData;
						registerResonanceFanOut();

						return () => {
							updatersRef.current.latent = null;
							registerResonanceFanOut();
						};
					}}
				/>
			</div>
		</Flex.Row>
	);
});

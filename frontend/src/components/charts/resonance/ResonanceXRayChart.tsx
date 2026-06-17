import { memo, useEffect, useRef } from "react";
import { SciChartReact } from "scichart-react";
import { appStore } from "#/collections/app";
import { initResonanceConstellationChart } from "#/components/charts/resonance/init-resonance-constellation-chart";
import { initResonanceUniverseHeatmap } from "#/components/charts/resonance/init-resonance-universe-heatmap";
import { initResonanceXRayChart } from "#/components/charts/resonance/init-resonance-xray-chart";
import { Flex } from "#/components/ui/flex";

type ResonanceFrameUpdater = (frame: Record<string, unknown>) => void;

export const ResonanceXRayChart = memo(function ResonanceXRayChart() {
	const updatersRef = useRef<{
		universeHeatmap: ResonanceFrameUpdater | null;
		xray: ResonanceFrameUpdater | null;
		constellation: ResonanceFrameUpdater | null;
	}>({
		universeHeatmap: null,
		xray: null,
		constellation: null,
	});

	const pendingFrameRef = useRef<Record<string, unknown> | null>(null);
	const animationFrameRef = useRef<number>(0);

	useEffect(() => {
		return () => {
			if (animationFrameRef.current !== 0) {
				cancelAnimationFrame(animationFrameRef.current);
			}
		};
	}, []);

	const registerResonanceFanOut = () => {
		appStore.actions.updateResonanceUpdater((frame) => {
			pendingFrameRef.current = frame;

			if (animationFrameRef.current !== 0) {
				return;
			}

			animationFrameRef.current = requestAnimationFrame(() => {
				animationFrameRef.current = 0;

				const pendingFrame = pendingFrameRef.current;

				if (pendingFrame === null) {
					return;
				}

				pendingFrameRef.current = null;
				updatersRef.current.universeHeatmap?.(pendingFrame);
				updatersRef.current.xray?.(pendingFrame);
				updatersRef.current.constellation?.(pendingFrame);
			});
		});
	};

	return (
		<Flex.Column className="h-full w-full min-h-0" gap={1} fullWidth fullHeight>
			<div className="h-[120px] w-full shrink-0 min-h-0">
				<SciChartReact
					style={{ height: "100%", width: "100%" }}
					initChart={initResonanceUniverseHeatmap}
					onInit={(result) => {
						updatersRef.current.universeHeatmap = result.addData;
						registerResonanceFanOut();

						return () => {
							updatersRef.current.universeHeatmap = null;
							registerResonanceFanOut();
						};
					}}
				/>
			</div>
			<Flex.Row className="min-h-0 flex-1" gap={1} fullWidth fullHeight>
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
						initChart={initResonanceConstellationChart}
						onInit={(result) => {
							updatersRef.current.constellation = result.addData;
							registerResonanceFanOut();

							return () => {
								updatersRef.current.constellation = null;
								registerResonanceFanOut();
							};
						}}
					/>
				</div>
			</Flex.Row>
		</Flex.Column>
	);
});

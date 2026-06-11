import { CircleAlertIcon } from "lucide-react";
import { memo } from "react";
import { SciChartReact } from "scichart-react";
import { appStore } from "#/collections/app";
import { drawSignalGauge } from "#/components/charts/confidence/draw-signal-gauge";
import { Card, CardPanel } from "#/components/ui/card";
import { Frame, FrameFooter } from "#/components/ui/frame";

export const SignalGauge = memo(function SignalGauge({
	source,
	label,
}: {
	source: string;
	label: string;
}) {
	return (
		<Frame className="flex h-full min-h-0 w-full flex-col">
			<Card className="min-h-0 flex-1 overflow-hidden">
				<CardPanel className="h-full w-full p-0">
					<SciChartReact
						initChart={drawSignalGauge}
						onInit={(result) => {
							appStore.actions.updateGaugeUpdater(source, result.addData);

							return () => appStore.actions.updateGaugeUpdater(source, null);
						}}
						style={{ height: "100%", width: "100%" }}
					/>
				</CardPanel>
			</Card>
			<FrameFooter className="shrink-0 py-1">
				<div className="flex gap-1 text-muted-foreground text-xs">
					<CircleAlertIcon className="size-3 h-lh shrink-0" />
					<p>{label}</p>
				</div>
			</FrameFooter>
		</Frame>
	);
});

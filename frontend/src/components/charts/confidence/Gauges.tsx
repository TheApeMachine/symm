import { CircleAlertIcon } from "lucide-react";
import { type MutableRefObject, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import { drawSignalGauge } from "#/components/charts/confidence/draw-signal-gauge";
import { Card, CardPanel } from "#/components/ui/card";
import { Frame, FrameFooter } from "#/components/ui/frame";

export type SignalGaugeBridge = {
	update: (confidence: number) => void;
	ready: boolean;
	pending: number[];
};

export const SignalGauge = ({
	bridgeRef,
	label,
}: {
	bridgeRef: MutableRefObject<SignalGaugeBridge>;
	label: string;
}) => {
	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawSignalGauge>) => {
			const bridge = bridgeRef.current;
			bridge.update = result.controls.update;
			bridge.ready = true;

			for (const confidence of bridge.pending) {
				bridge.update(confidence);
			}

			bridge.pending = [];

			return () => {
				bridge.update = () => {};
				bridge.ready = false;
				bridge.pending = [];
			};
		},
		[bridgeRef],
	);

	return (
		<Frame className="w-full h-full">
			<Card className="h-full w-full overflow-hidden">
				<CardPanel className="h-full w-full p-0">
					<SciChartReact
						initChart={drawSignalGauge}
						onInit={onInit}
						style={{ height: "100%", width: "100%" }}
					/>
				</CardPanel>
			</Card>
			<FrameFooter className="py-1">
				<div className="flex gap-1 text-muted-foreground text-xs">
					<CircleAlertIcon className="size-3 h-lh shrink-0" />
					<p>{label}</p>
				</div>
			</FrameFooter>
		</Frame>
	);
};

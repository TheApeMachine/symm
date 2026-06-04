import { CircleAlertIcon } from "lucide-react";
import { useCallback, useState } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import { drawSignalGauge } from "#/components/charts/confidence/draw-signal-gauge";
import {
	confidenceFromGaugePayload,
	gaugePayloadEntries,
} from "#/components/charts/confidence/gauge-payload";
import { Card, CardPanel } from "#/components/ui/card";
import { Frame, FrameFooter } from "#/components/ui/frame";

export type SignalGaugeBridge = {
	update: (payload: Record<string, unknown>) => void;
	ready: boolean;
	pending: Record<string, unknown>[];
	latest: Record<string, unknown>;
};

const SignalGaugeTooltip = ({
	payload,
}: {
	payload: Record<string, unknown>;
}) => {
	const entries = gaugePayloadEntries(payload);

	if (entries.length === 0) {
		return null;
	}

	return (
		<dl className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 font-mono leading-tight">
			{entries.map(([key, value]) => (
				<div className="contents" key={key}>
					<dt className="text-muted-foreground">{key}</dt>
					<dd className="text-foreground tabular-nums">{value}</dd>
				</div>
			))}
		</dl>
	);
};

export const SignalGauge = ({
	bridge,
	label,
}: {
	bridge: SignalGaugeBridge;
	label: string;
}) => {
	const [tooltipPayload, setTooltipPayload] = useState<Record<
		string,
		unknown
	> | null>(null);

	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawSignalGauge>) => {
			bridge.latest = {};

			bridge.update = (nextPayload) => {
				bridge.latest = nextPayload;
				result.controls.update(confidenceFromGaugePayload(nextPayload));
			};

			bridge.ready = true;

			for (const pendingPayload of bridge.pending) {
				bridge.update(pendingPayload);
			}

			bridge.pending = [];

			return () => {
				bridge.update = () => {};
				bridge.ready = false;
				bridge.pending = [];
				bridge.latest = {};
			};
		},
		[bridge],
	);

	const showTooltip =
		tooltipPayload !== null && gaugePayloadEntries(tooltipPayload).length > 0;

	return (
		<Frame className="w-full h-full">
			<Card className="h-full w-full overflow-hidden">
				<CardPanel className="h-full w-full p-0">
					<div
						className="relative h-full w-full"
						onPointerEnter={() => setTooltipPayload({ ...bridge.latest })}
						onPointerLeave={() => setTooltipPayload(null)}
					>
						<SciChartReact
							initChart={drawSignalGauge}
							onInit={onInit}
							style={{ height: "100%", width: "100%" }}
						/>
						{showTooltip ? (
							<div
								className="pointer-events-none absolute inset-x-0 top-2 z-10 mx-auto w-max max-w-[min(100%,14rem)] rounded-md border bg-popover px-2 py-1 text-popover-foreground text-xs shadow-md/5"
								role="tooltip"
							>
								<SignalGaugeTooltip payload={tooltipPayload} />
							</div>
						) : null}
					</div>
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

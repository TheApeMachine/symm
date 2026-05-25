import { memo, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import { drawSignalGauge } from "#/components/symm/draw-signal-gauge";
import {
	SIGNAL_LABELS,
	SIGNAL_SOURCES,
	type SignalSource,
} from "#/lib/symm/signal-confidence";
import { registerSignalGauge } from "#/lib/symm/feed";
import "#/lib/symm/scichart-setup";

type SignalGaugeProps = {
	source: SignalSource;
};

const SignalGauge = memo(function SignalGauge({ source }: SignalGaugeProps) {
	const initChart = useCallback((rootElement: string | HTMLDivElement) => {
		if (typeof rootElement === "string") {
			throw new Error("drawSignalGauge requires an HTMLDivElement root");
		}

		return drawSignalGauge(rootElement);
	}, []);

	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawSignalGauge>) =>
			registerSignalGauge(source, (needlePercent, confidence) => {
				result.controls.update(needlePercent, confidence);
			}),
		[source],
	);

	return (
		<div className="flex min-h-0 min-w-0 flex-col overflow-hidden">
			<span className="shrink-0 truncate px-1 text-[9px] font-medium tracking-wide text-(--dash-muted)">
				{SIGNAL_LABELS[source]}
			</span>
			<SciChartReact
				initChart={initChart}
				onInit={onInit}
				className="min-h-0 w-full flex-1"
				innerContainerProps={{ className: "h-full w-full" }}
			/>
		</div>
	);
});

export const Gauges = () => {
	return (
		<div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel)">
			<div className="grid min-h-0 flex-1 grid-cols-4 gap-1 p-1">
				{SIGNAL_SOURCES.map((source) => (
					<SignalGauge key={source} source={source} />
				))}
			</div>
		</div>
	);
};

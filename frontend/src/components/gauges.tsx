import { memo, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import { ConfidenceDataProvider } from "#/components/symm/confidence-data-provider";
import { drawSignalGauge } from "#/components/symm/draw-signal-gauge";
import {
	SIGNAL_LABELS,
	SIGNAL_SOURCES,
	type SignalSource,
} from "#/lib/symm/signal-confidence";
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
		(result: TResolvedReturnType<typeof drawSignalGauge>) => {
			const latest = ConfidenceDataProvider.snapshot().get(source);

			if (latest !== undefined) {
				result.controls.update(latest);
			}

			const unregister = ConfidenceDataProvider.registerSource(
				source,
				(confidence) => {
					result.controls.update(confidence);
				},
			);

			return () => {
				unregister();
				result.controls.dispose();
			};
		},
		[source],
	);

	return (
		<SciChartReact
			initChart={initChart}
			onInit={onInit}
			className="min-h-0 w-full flex-1"
			innerContainerProps={{ className: "h-full w-full" }}
		/>
	);
});

export const Gauges = () => {
	return (
		<div className="flex h-full min-h-0 flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) p-1">
			<div className="grid min-h-0 flex-1 grid-cols-4 grid-rows-2 gap-1">
				{SIGNAL_SOURCES.map((source: SignalSource) => (
					<div
						key={source}
						className="flex min-h-0 min-w-0 flex-col overflow-hidden"
					>
						<small className="truncate px-0.5 text-center text-[9px] text-(--dash-muted)">
							{SIGNAL_LABELS[source]}
						</small>
						<SignalGauge source={source} />
					</div>
				))}
			</div>
		</div>
	);
};

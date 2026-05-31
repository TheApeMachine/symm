import { memo, useCallback, useState } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import {
	ConfidenceDataProvider,
	type ConfidenceFactor,
	formatConfidenceFactor,
} from "#/components/symm/confidence-data-provider";
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

const GaugeFactorTooltip = ({ factors }: { factors: ConfidenceFactor[] }) => {
	if (factors.length === 0) {
		return null;
	}

	return (
		<div className="pointer-events-none absolute bottom-1 left-1 right-1 z-10 rounded border border-(--dash-border) bg-(--dash-panel)/95 px-1 py-0.5 text-[8px] leading-tight text-(--dash-muted) shadow-sm">
			{factors.map((factor) => (
				<div key={factor.name} className="truncate font-mono">
					{formatConfidenceFactor(factor)}
				</div>
			))}
		</div>
	);
};

const SignalGauge = memo(function SignalGauge({ source }: SignalGaugeProps) {
	const [factors, setFactors] = useState<ConfidenceFactor[]>([]);
	const [showFactors, setShowFactors] = useState(false);

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
				result.controls.update(latest.confidence);
				setFactors(latest.factors ?? []);
			}

			const unregister = ConfidenceDataProvider.registerSource(
				source,
				(row) => {
					result.controls.update(row.confidence);
					setFactors(row.factors ?? []);
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
		<div
			className="relative min-h-0 flex-1"
			onMouseEnter={() => setShowFactors(true)}
			onMouseLeave={() => setShowFactors(false)}
		>
			<SciChartReact
				initChart={initChart}
				onInit={onInit}
				className="h-full w-full min-h-0"
				innerContainerProps={{ className: "h-full w-full" }}
			/>
			{showFactors ? <GaugeFactorTooltip factors={factors} /> : null}
		</div>
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

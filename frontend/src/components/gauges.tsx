import { memo, useCallback, useState } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import {
	ConfidenceDataProvider,
	type ConfidenceFactor,
	formatConfidenceFactor,
} from "#/components/symm/confidence-data-provider";
import { drawSignalGauge } from "#/components/symm/draw-signal-gauge";
import { formatSignalConfidence } from "#/lib/symm/signal-confidence";
import {
	defaultLayoutDocument,
	gaugeLabelFor,
	type LayoutPanel,
} from "#/lib/symm/layout-schema";
import "#/lib/symm/scichart-setup";

type SignalGaugeProps = {
	source: string;
};

const GaugeFactorTooltip = ({
	confidence,
	factors,
}: {
	confidence: number;
	factors: ConfidenceFactor[];
}) => {
	return (
		<div className="pointer-events-none absolute bottom-1 left-1 right-1 z-10 rounded border border-(--dash-border) bg-(--dash-panel)/95 px-1 py-0.5 text-[8px] leading-tight text-(--dash-muted) opacity-0 shadow-sm transition-opacity group-hover:opacity-100">
			<div className="truncate font-mono">
				snr={formatSignalConfidence(confidence)}
			</div>
			{factors.map((factor) => (
				<div key={factor.name} className="truncate font-mono">
					{formatConfidenceFactor(factor)}
				</div>
			))}
		</div>
	);
};

const SignalGauge = memo(function SignalGauge({ source }: SignalGaugeProps) {
	const [confidence, setConfidence] = useState(0);
	const [factors, setFactors] = useState<ConfidenceFactor[]>([]);
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
				setConfidence(latest.confidence);
				setFactors(latest.factors ?? []);
			}

			const unregister = ConfidenceDataProvider.registerSource(
				source,
				(row) => {
					result.controls.update(row.confidence);
					setConfidence(row.confidence);
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
		<div className="group relative min-h-0 flex-1">
			<SciChartReact
				initChart={initChart}
				onInit={onInit}
				className="h-full w-full min-h-0"
				innerContainerProps={{ className: "h-full w-full" }}
			/>
			<GaugeFactorTooltip confidence={confidence} factors={factors} />
		</div>
	);
});

export const Gauges = ({ panel }: { panel?: LayoutPanel }) => {
	const gaugePanel =
		panel ??
		defaultLayoutDocument().panels.find((entry) => entry.type === "gauge_grid");

	const sources = gaugePanel?.sources ?? [];

	return (
		<div className="flex h-full min-h-0 flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) p-1">
			<div className="grid min-h-0 flex-1 grid-cols-4 grid-rows-2 gap-1">
				{sources.map((source) => (
					<div
						key={source}
						className="flex min-h-0 min-w-0 flex-col overflow-hidden"
					>
						<small className="truncate px-0.5 text-center text-[9px] text-(--dash-muted)">
							{gaugeLabelFor(gaugePanel ?? { type: "gauge_grid" }, source)}
						</small>
						<SignalGauge source={source} />
					</div>
				))}
			</div>
		</div>
	);
};

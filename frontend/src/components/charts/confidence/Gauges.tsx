import { memo, useCallback, useState } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import {
	type ConfidenceFactor,
	formatConfidenceFactor,
} from "#/components/charts/confidence/confidence-data-provider";
import { drawSignalGauge } from "#/components/charts/confidence/draw-signal-gauge";
import { TelemetryManifestPlaceholder } from "#/components/dashboard/TelemetryManifestPlaceholder";
import {
	defaultLayoutDocument,
	GAUGE_GRID_CAPACITY,
	gaugeGridLayout,
	gaugeLabelFor,
	gaugeSourcesFor,
	hasGaugeSources,
	type LayoutPanel,
} from "#/lib/symm/layout-schema";
import { formatSignalConfidence } from "#/lib/symm/signal-confidence";
import { useSymmTelemetryStores } from "#/lib/symm/telemetry-context";
import "#/lib/symm/scichart-setup";

type GaugeVariant = "grid" | "strip";

type SignalGaugeProps = {
	source: string;
};

const GaugeFactorTooltip = ({
	confidence,
	snr,
	factors,
}: {
	confidence: number;
	snr?: number;
	factors: ConfidenceFactor[];
}) => {
	return (
		<div className="pointer-events-none absolute bottom-1 left-1 right-1 z-10 rounded border border-(--dash-border) bg-(--dash-panel)/95 px-1 py-0.5 text-[8px] leading-tight text-(--dash-muted) opacity-0 shadow-sm transition-opacity group-hover:opacity-100">
			<div className="truncate font-mono">
				clarity={formatSignalConfidence(confidence)}
			</div>
			{snr !== undefined ? (
				<div className="truncate font-mono">snr={snr.toFixed(2)}σ</div>
			) : null}
			{factors.map((factor) => (
				<div key={factor.name} className="truncate font-mono">
					{formatConfidenceFactor(factor)}
				</div>
			))}
		</div>
	);
};

const SignalGauge = memo(function SignalGauge({ source }: SignalGaugeProps) {
	const stores = useSymmTelemetryStores();
	const [confidence, setConfidence] = useState(0);
	const [snr, setSnr] = useState<number | undefined>(undefined);
	const [factors, setFactors] = useState<ConfidenceFactor[]>([]);
	const initChart = useCallback((rootElement: string | HTMLDivElement) => {
		if (typeof rootElement === "string") {
			throw new Error("drawSignalGauge requires an HTMLDivElement root");
		}

		return drawSignalGauge(rootElement);
	}, []);

	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawSignalGauge>) => {
			const confidenceStore = stores.confidence;
			const unregister = confidenceStore.registerSource(source, (row) => {
				result.controls.update(row.confidence);
				setConfidence(row.confidence);
				setSnr(row.snr);
				setFactors(row.factors ?? []);
			});

			return () => {
				unregister();
				result.controls.dispose();
			};
		},
		[source, stores.confidence],
	);

	return (
		<div className="group relative min-h-0 flex-1">
			<SciChartReact
				initChart={initChart}
				onInit={onInit}
				className="h-full w-full min-h-0"
				innerContainerProps={{ className: "h-full w-full" }}
			/>
			<GaugeFactorTooltip confidence={confidence} snr={snr} factors={factors} />
		</div>
	);
});

type GaugesProps = {
	panel?: LayoutPanel;
	variant?: GaugeVariant;
};

export const Gauges = ({ panel, variant = "grid" }: GaugesProps) => {
	const fallbackPanel = defaultLayoutDocument().panels.find(
		(entry) =>
			entry.type === (variant === "strip" ? "gauge_strip" : "gauge_grid"),
	);
	const gaugePanel = panel ?? fallbackPanel;
	const sources = gaugeSourcesFor(gaugePanel);
	const labelPanel = gaugePanel ?? {
		type:
			variant === "strip" ? ("gauge_strip" as const) : ("gauge_grid" as const),
	};

	if (!hasGaugeSources(gaugePanel)) {
		if (variant === "strip") {
			return null;
		}

		return (
			<div className="flex h-full min-h-0 flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) p-1">
				<TelemetryManifestPlaceholder />
			</div>
		);
	}

	if (variant === "strip") {
		return (
			<div className="dashboard-gauge-strip flex h-full min-h-0 flex-col gap-1 overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) p-1">
				{sources.map((source) => (
					<div
						key={source}
						className="flex min-h-0 flex-1 flex-col overflow-hidden"
					>
						<small className="truncate px-0.5 text-center text-[9px] text-(--dash-muted)">
							{gaugeLabelFor(labelPanel, source)}
						</small>
						<SignalGauge source={source} />
					</div>
				))}
			</div>
		);
	}

	const cappedSources = sources.slice(0, GAUGE_GRID_CAPACITY);
	const { columns, rows } = gaugeGridLayout(cappedSources);

	return (
		<div className="flex h-full min-h-0 flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) p-1">
			<div
				className="grid min-h-0 flex-1 gap-1"
				style={{
					gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
					gridTemplateRows: `repeat(${rows}, minmax(0, 1fr))`,
				}}
			>
				{cappedSources.map((source) => (
					<div
						key={source}
						className="flex min-h-0 min-w-0 flex-col overflow-hidden"
					>
						<small className="truncate px-0.5 text-center text-[9px] text-(--dash-muted)">
							{gaugeLabelFor(labelPanel, source)}
						</small>
						<SignalGauge source={source} />
					</div>
				))}
			</div>
		</div>
	);
};

import { memo, useCallback } from "react";
import { SciChartReact } from "scichart-react";

import { EnginePulseDataProvider } from "#/components/symm/engine-pulse-data-provider";
import { initEnginePulseChart } from "#/components/symm/init-engine-pulse";
import {
	useSymmConnected,
	useSymmEnginePulse,
} from "#/lib/symm/use-dashboard-data";
import "#/lib/symm/scichart-setup";

type EnginePulseChartProps = {
	className?: string;
};

/** Live average prediction vs running error — one point per engine_pulse tick. */
export const EnginePulseChart = memo(function EnginePulseChart({
	className = "",
}: EnginePulseChartProps) {
	const initChart = useCallback((rootElement: string | HTMLDivElement) => {
		if (typeof rootElement === "string") {
			throw new Error("initEnginePulseChart requires an HTMLDivElement root");
		}

		return initEnginePulseChart(rootElement);
	}, []);

	const onInit = useCallback(
		(result: Awaited<ReturnType<typeof initEnginePulseChart>>) =>
			EnginePulseDataProvider.registerSink(result.appendPulse),
		[],
	);

	return (
		<div
			className={`flex min-h-[180px] flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) ${className}`}
		>
			<EnginePulseHeader />
			<SciChartReact
				initChart={initChart}
				onInit={onInit}
				className="min-h-0 w-full flex-1"
				innerContainerProps={{ className: "h-full w-full" }}
			/>
			<p className="shrink-0 border-t border-(--dash-border) px-2 py-0.5 text-[9px] text-(--dash-muted)">
				Prediction · Error — symbol averages per rescore tick
			</p>
		</div>
	);
});

const EnginePulseHeader = memo(function EnginePulseHeader() {
	const connected = useSymmConnected();
	const pulse = useSymmEnginePulse();

	return (
		<div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-(--dash-border) px-2 py-1.5">
			<span className="text-xs font-semibold tracking-wide">Engine pulse</span>
			<span className="text-[10px] text-(--dash-muted)">
				{connected ? `tick #${pulse?.seq ?? 0}` : "Offline"}
			</span>
			{pulse ? (
				<div className="ml-auto flex flex-wrap gap-3 text-[10px] tabular-nums text-(--dash-muted)">
					<span>
						pred{" "}
						<span className="font-medium text-(--dash-text)">
							{formatReturn(pulse.avg_prediction)}
						</span>
					</span>
					<span>
						err{" "}
						<span className="font-medium text-(--dash-text)">
							{formatReturn(pulse.avg_error)}
						</span>
					</span>
					<span>
						syms{" "}
						<span className="font-medium text-(--dash-text)">
							{pulse.forecast_symbols ?? pulse.candidates ?? 0}
						</span>
					</span>
					<PulseMetric
						label="quotes"
						value={pulse.ticker_ready}
						total={pulse.symbols_total}
					/>
					<span>
						sig{" "}
						<span className="font-medium text-(--dash-text)">
							{pulse.measurements}
						</span>
					</span>
				</div>
			) : null}
		</div>
	);
});

function formatReturn(value: number | undefined) {
	if (value === undefined || !Number.isFinite(value)) {
		return "—";
	}

	return value.toFixed(4);
}

function PulseMetric({
	label,
	value,
	total,
	warm,
}: {
	label: string;
	value?: number;
	total?: number;
	warm?: boolean;
}) {
	if (value === undefined && total === undefined) {
		return null;
	}

	return (
		<span>
			{label}{" "}
			<span className="font-medium text-(--dash-text)">{value ?? 0}</span>
			{total !== undefined ? (
				<span>
					{warm ? "+" : "/"}
					{total}
				</span>
			) : null}
		</span>
	);
}

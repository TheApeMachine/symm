import { memo, useCallback, useEffect, useRef } from "react";
import { SciChartReact } from "scichart-react";

import {
	initEnginePulseChart,
	type EnginePulseInitResult,
} from "#/lib/symm/engine-pulse-controller";
import {
	replayEnginePulseHistory,
	subscribeTradeChart,
	unsubscribeTradeChart,
} from "#/lib/symm/feed";
import { engineStore } from "#/lib/symm/stores/engine-store";
import { useSymmConnected, useSymmEnginePulse } from "#/lib/symm/use-symm-ui";
import "#/lib/symm/scichart-setup";

type EnginePulseChartProps = {
	className?: string;
};

/** Live average prediction vs running error — one point per engine_pulse tick. */
export const EnginePulseChart = memo(function EnginePulseChart({
	className = "",
}: EnginePulseChartProps) {
	const chartRef = useRef<EnginePulseInitResult | null>(null);
	const lastSeqRef = useRef(0);
	const pulse = useSymmEnginePulse();

	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) => initEnginePulseChart(rootElement),
		[],
	);

	const onInit = useCallback((result: EnginePulseInitResult) => {
		chartRef.current = result;
		replayEnginePulseHistory((entry) => result.appendPulse(entry));
		lastSeqRef.current = engineStore.state.pulseLog[0]?.seq ?? 0;
	}, []);

	const onDelete = useCallback((result?: EnginePulseInitResult) => {
		result?.dispose();
		chartRef.current = null;
		lastSeqRef.current = 0;
	}, []);

	useEffect(() => {
		if (!pulse || !chartRef.current || pulse.seq <= lastSeqRef.current) {
			return;
		}

		chartRef.current.appendPulse(pulse);
		lastSeqRef.current = pulse.seq;
	}, [pulse]);

	return (
		<div
			className={`flex min-h-[180px] flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) ${className}`}
		>
			<EnginePulseHeader />
			<SciChartReact
				initChart={initChart}
				onInit={onInit}
				onDelete={onDelete}
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
}: {
	label: string;
	value?: number;
	total?: number;
}) {
	if (value === undefined && total === undefined) {
		return null;
	}

	return (
		<span>
			{label}{" "}
			<span className="font-medium text-(--dash-text)">{value ?? 0}</span>
			{total !== undefined ? <span>/{total}</span> : null}
		</span>
	);
}

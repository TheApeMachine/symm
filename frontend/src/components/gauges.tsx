import { memo, useCallback, useEffect, useRef } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import { drawSignalGauge } from "#/components/symm/draw-signal-gauge";
import {
	confidenceToGaugePercent,
	SIGNAL_LABELS,
	SIGNAL_SOURCES,
	type SignalSource,
} from "#/lib/symm/signal-confidence";
import {
	useSymmConnected,
	useSymmEntryLine,
	useSymmSignalConfidences,
} from "#/lib/symm/use-symm-ui";
import "#/lib/symm/scichart-setup";

type SignalGaugeProps = {
	source: SignalSource;
	confidence: number;
};

const SignalGauge = memo(function SignalGauge({
	source,
	confidence,
}: SignalGaugeProps) {
	const controlsRef =
		useRef<TResolvedReturnType<typeof drawSignalGauge>["controls"] | null>(
			null,
		);
	const readingRef = useRef(confidence);
	readingRef.current = confidence;

	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) => {
			if (typeof rootElement === "string") {
				throw new Error("drawSignalGauge requires an HTMLDivElement root");
			}

			return drawSignalGauge(rootElement);
		},
		[],
	);

	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawSignalGauge>) => {
			controlsRef.current = result.controls;
			result.controls.update(
				confidenceToGaugePercent(readingRef.current),
				readingRef.current,
			);

			return () => {
				controlsRef.current = null;
				result.controls.dispose();
			};
		},
		[],
	);

	useEffect(() => {
		controlsRef.current?.update(
			confidenceToGaugePercent(confidence),
			confidence,
		);
	}, [confidence]);

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
	const connected = useSymmConnected();
	const entryLine = useSymmEntryLine();
	const confidences = useSymmSignalConfidences();

	return (
		<div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel)">
			<div className="flex shrink-0 items-center justify-between border-b border-(--dash-border) px-2 py-1">
				<span className="text-xs font-semibold tracking-wide">Signals</span>
				<span className="text-[10px] text-(--dash-muted)">
					{connected
						? entryLine.line > 0
							? `line ${entryLine.line.toFixed(3)}`
							: "Warming"
						: "Offline"}
				</span>
			</div>
			<div className="grid min-h-0 flex-1 grid-cols-4 gap-1 p-1">
				{SIGNAL_SOURCES.map((source) => (
					<SignalGauge
						key={source}
						source={source}
						confidence={confidences[source]}
					/>
				))}
			</div>
		</div>
	);
};

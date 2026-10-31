import type { MutableRefObject } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import { SciChartReact } from "scichart-react";
import { drawOpenChoiceChart } from "./open-choice-draw";

type ChartApi = {
	setIsGridLayoutMode: (value: boolean) => void;
	syncSeries: (lots: Record<string, unknown>[]) => void;
};

/** Client-only SciChart surface; keep scichart off the SSR import graph. */
export const OpenChoiceSciChart = ({
	lots,
	trailGen,
	apiRef,
}: {
	lots: Record<string, unknown>[];
	trailGen: number;
	apiRef: MutableRefObject<ChartApi | null>;
}) => {
	const [ready, setReady] = useState(false);
	const lotsRef = useRef(lots);
	lotsRef.current = lots;
	const lotIds = lots
		.map((row) => String(row.id ?? row.symbol))
		.sort()
		.join("|");

	const initChart = useCallback(
		(rootElement: string | HTMLDivElement) => drawOpenChoiceChart(rootElement),
		[],
	);

	const onInit = useCallback(
		(api: ChartApi) => {
			apiRef.current = api;
			api.syncSeries(lotsRef.current);
			setReady(true);
			return () => {
				apiRef.current = null;
				setReady(false);
			};
		},
		[apiRef],
	);

	// biome-ignore lint/correctness/useExhaustiveDependencies: trailGen/lotIds are change signals; lots are read via lotsRef.
	useEffect(() => {
		if (!ready || !apiRef.current) return;
		void lotIds;
		apiRef.current.syncSeries(lotsRef.current);
	}, [trailGen, lotIds, ready, apiRef]);

	return (
		<SciChartReact
			initChart={initChart}
			onInit={onInit}
			onInitError={() => setReady(false)}
			style={{ width: "100%", height: "100%" }}
			fallback={
				<div className="flex h-full w-full items-center justify-center text-[11px] text-(--f4)">
					loading chart…
				</div>
			}
		/>
	);
};

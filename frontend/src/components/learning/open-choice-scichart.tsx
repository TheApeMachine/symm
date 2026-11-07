import { useEffect, useRef, useState } from "react";
import { SciChartReact } from "scichart-react";
import { signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { Position } from "#/collections/types";
import {
	drawOpenChoiceChart,
	type OpenChoiceChartApi,
	type OpenChoiceLegendItem,
} from "./open-choice-draw";

/** Client-only SciChart surface; store → series, no React trail buffers. */
export const OpenChoiceSciChart = ({
	onLegend,
	onReady,
}: {
	onLegend: (items: OpenChoiceLegendItem[]) => void;
	onReady?: (api: OpenChoiceChartApi) => void;
}) => {
	const apiRef = useRef<OpenChoiceChartApi | null>(null);
	const onLegendRef = useRef(onLegend);
	const onReadyRef = useRef(onReady);
	onLegendRef.current = onLegend;
	onReadyRef.current = onReady;
	const [ready, setReady] = useState(false);

	useEffect(() => {
		if (!ready || !apiRef.current) return;

		let raf = 0;

		const push = () => {
			const api = apiRef.current;
			if (!api) return;

			const items = api.sync(
				signals.positions.state as Record<string, Position>,
				bookAtom.get(),
			);

			// Round display figures so React legend chrome does not re-render
			// on every sub-cent mark tick.
			onLegendRef.current(
				items.map((item) => ({
					...item,
					pnlMoney:
						item.pnlMoney === undefined
							? undefined
							: Math.round(item.pnlMoney * 100) / 100,
					pnlPct:
						item.pnlPct === undefined
							? undefined
							: Math.round(item.pnlPct * 10) / 10,
					stopRoomPct:
						item.stopRoomPct === undefined
							? undefined
							: Math.round(item.stopRoomPct * 10) / 10,
					stopProximity:
						item.stopProximity === undefined
							? undefined
							: Math.round(item.stopProximity * 100) / 100,
				})),
			);
		};

		const schedule = () => {
			if (raf !== 0) return;
			raf = requestAnimationFrame(() => {
				raf = 0;
				push();
			});
		};

		push();

		const positionsSub = signals.positions.subscribe(schedule);
		const bookSub = bookAtom.subscribe(schedule);

		return () => {
			if (raf !== 0) cancelAnimationFrame(raf);
			positionsSub.unsubscribe();
			bookSub.unsubscribe();
		};
	}, [ready]);

	return (
		<SciChartReact
			initChart={drawOpenChoiceChart}
			onInit={(api: OpenChoiceChartApi) => {
				apiRef.current = api;
				onReadyRef.current?.(api);
				onLegendRef.current(
					api.sync(
						signals.positions.state as Record<string, Position>,
						bookAtom.get(),
					),
				);
				setReady(true);
				return () => {
					apiRef.current = null;
					setReady(false);
				};
			}}
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

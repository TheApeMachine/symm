import { useRef } from "react";
import { SciChartReact } from "scichart-react";
import { signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { Position } from "#/collections/types";
import {
	drawOpenChoiceChart,
	type OpenChoiceChartApi,
	type OpenChoiceLegendItem,
} from "./open-choice-draw";

const roundLegend = (items: OpenChoiceLegendItem[]): OpenChoiceLegendItem[] =>
	items.map((item) => ({
		...item,
		pnlMoney:
			item.pnlMoney === undefined
				? undefined
				: Math.round(item.pnlMoney * 100) / 100,
		grossMoney:
			item.grossMoney === undefined
				? undefined
				: Math.round(item.grossMoney * 100) / 100,
		feeMoney:
			item.feeMoney === undefined
				? undefined
				: Math.round(item.feeMoney * 100) / 100,
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
	}));

/** Client-only SciChart surface; paints from signals.positions snapshots. */
export const OpenChoiceSciChart = ({
	onLegend,
	onReady,
}: {
	onLegend: (items: OpenChoiceLegendItem[]) => void;
	onReady?: (api: OpenChoiceChartApi) => void;
}) => {
	const onLegendRef = useRef(onLegend);
	const onReadyRef = useRef(onReady);
	onLegendRef.current = onLegend;
	onReadyRef.current = onReady;

	return (
		<SciChartReact
			initChart={drawOpenChoiceChart}
			onInit={(api: OpenChoiceChartApi) => {
				api.setBook(bookAtom.get());
				onReadyRef.current?.(api);

				const paint = (state: Record<string, Position> | null) => {
					onLegendRef.current(
						roundLegend(api.push(state ?? {})),
					);
				};

				paint(signals.positions.state as Record<string, Position>);

				const posSub = signals.positions.subscribe((state) => {
					paint(state as Record<string, Position>);
				});

				const bookSub = bookAtom.subscribe((book) => {
					api.setBook(book);
					paint(signals.positions.state as Record<string, Position>);
				});

				return () => {
					posSub.unsubscribe();
					bookSub.unsubscribe();
				};
			}}
			style={{ width: "100%", height: "100%" }}
			fallback={
				<div className="flex h-full w-full items-center justify-center text-[11px] text-(--f4)">
					loading chart…
				</div>
			}
		/>
	);
};

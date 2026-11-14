import { useRef } from "react";
import { SciChartReact } from "scichart-react";
import { bindOpenChoice } from "#/collections/app";
import { bookAtom } from "#/collections/book";
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

/** Client-only SciChart surface; hub frames push into series directly. */
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

				bindOpenChoice({
					push: (marks, opts) => {
						onLegendRef.current(roundLegend(api.push(marks, opts)));
					},
				});

				const bookSub = bookAtom.subscribe((book) => {
					api.setBook(book);
					onLegendRef.current([]);
				});

				return () => {
					bookSub.unsubscribe();
					bindOpenChoice(null);
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

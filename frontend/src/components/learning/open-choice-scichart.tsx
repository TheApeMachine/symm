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

		const push = () => {
			const api = apiRef.current;
			if (!api) return;
			onLegendRef.current(
				api.sync(
					signals.positions.state as Record<string, Position>,
					bookAtom.get(),
				),
			);
		};

		push();

		const positionsSub = signals.positions.subscribe(push);
		const bookSub = bookAtom.subscribe(push);

		return () => {
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

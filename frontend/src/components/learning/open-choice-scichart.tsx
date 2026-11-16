import { useRef } from "react";
import { FastLineRenderableSeries, XyDataSeries } from "scichart";
import { SciChartReact } from "scichart-react";
import { signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { Position } from "#/collections/types";
import {
	drawOpenChoiceChart,
	type OpenChoiceChartApi,
} from "./open-choice-draw";
import { strokeFor } from "./open-choice-stroke";

/** Client-only SciChart surface — subscribe and append pnl_pct lines. */
export const OpenChoiceSciChart = ({
	onReady,
}: {
	onReady?: (api: OpenChoiceChartApi) => void;
}) => {
	const onReadyRef = useRef(onReady);
	onReadyRef.current = onReady;
	const tick = useRef(0);

	return (
		<SciChartReact
			initChart={drawOpenChoiceChart}
			onInit={(api: OpenChoiceChartApi) => {
				onReadyRef.current?.(api);

				const updateChart = () => {
					tick.current++;
					const x = tick.current;
					const seen: string[] = [];
					const currentBook = bookAtom.get();
					const positions = signals.positions.state;

					for (const pos of Object.values(positions)) {
						const position = pos as Position;
						const id = position.id;
						if (!id || position.closed_at) continue;
						const tier = position.tier_key || "paper";
						if (tier !== currentBook) continue;

						let series = api.sciChartSurface.renderableSeries.getById(id);

						if (!series) {
							series = new FastLineRenderableSeries(api.wasmContext, {
								id,
								dataSeries: new XyDataSeries(api.wasmContext, {
									fifoCapacity: 90,
								}),
								stroke: strokeFor(id),
								strokeThickness: 2,
							});
							api.sciChartSurface.renderableSeries.add(series);
						}

						const y = parseFloat(String(position.pnl_pct ?? ""));
						if (!Number.isFinite(y)) continue;

						(series.dataSeries as XyDataSeries).append(x, y);
						seen.push(id);
					}

					for (const series of api.sciChartSurface.renderableSeries.asArray()) {
						const id = series.id as string;
						if (id && !seen.includes(id)) {
							api.sciChartSurface.renderableSeries.remove(series);
						}
					}

					api.sciChartSurface.invalidateElement();
				};

				const subPositions = signals.positions.subscribe(updateChart);
				const subBook = bookAtom.subscribe(updateChart);

				return () => {
					subPositions.unsubscribe();
					subBook.unsubscribe();
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

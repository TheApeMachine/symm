import { useRef } from "react";
import { FastLineRenderableSeries, XyDataSeries } from "scichart";
import { SciChartReact } from "scichart-react";
import { signals } from "#/collections/app";
import type { Position } from "#/collections/types";
import {
	drawOpenChoiceChart,
	type OpenChoiceChartApi,
} from "./open-choice-draw";

const STROKES = [
	"#79D2C6",
	"#FF7DFD",
	"#5B88FF",
	"#D1765F",
	"#F5F77F",
	"#F8CFF7",
	"#6CA261",
	"#F1B470",
	"#B3A0D4",
	"#80F078",
];

const strokeFor = (id: string) => {
	let h = 0;
	for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) | 0;
	return STROKES[Math.abs(h) % STROKES.length];
};

/** Client-only SciChart surface — subscribe and append pnl_pct. */
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

				const sub = signals.positions.subscribe((positions) => {
					tick.current++;
					const x = tick.current;
					const seen: string[] = [];

					for (const pos of Object.values(positions)) {
						const position = pos as Position;
						const id = position.id;
						if (!id) continue;

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
				});

				return () => {
					sub.unsubscribe();
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

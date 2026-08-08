import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { appStore } from "#/collections/app";
import type { ResonanceFrame } from "#/collections/types";
import { drawPredictiveCodingChart } from "#/components/charts/prediction/canvas";
import { resizeCanvas } from "#/components/terminal/canvas";
import type { JSONSerializable } from "#/components/ui/paint";
import { registerPainter } from "#/providers/ws-stores";

/*
How many settled epochs the chart keeps. The hierarchy is a time series — a
layer's reconstruction error only means something against the errors before it —
so the canvas retains its own bounded history rather than redrawing one frame.
*/
const HISTORY = 240;

const asFrames = (updates: JSONSerializable): ResonanceFrame[] =>
	Array.isArray(updates) ? (updates as unknown as ResonanceFrame[]) : [];

/*
PredictiveCodingCanvas draws the settled predictive-coding hierarchy and the
supervised return head beside it.

Each layer gets its own lane on its own scale, because the states differ in
width and in magnitude: the wire carries a 29-wide sensory layer, a 58-wide
middle layer and a 29-wide context layer, and a forward return around 1e-8.
Plotting those against one shared axis is what produced a flat band and a solid
block — every element of the small vector collapsed onto the zero line while the
largest saturated its lane.

A laid-out multi-lane chart cannot be expressed as data attributes, so this
registers its own painter, the same arrangement the market graph and the cortex
tree use.
*/
export const PredictiveCodingCanvas = ({
	className,
}: {
	className?: string;
}) => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const canvasRef = useRef<HTMLCanvasElement>(null);
	const history = useRef<ResonanceFrame[]>([]);

	useEffect(() => {
		/*
			Retained history belongs to one carrier. Re-pointing the chart at another
			symbol starts a new series rather than splicing two unrelated ones.
		*/
		history.current = [];

		const draw = () => {
			const canvas = canvasRef.current;

			if (canvas === null) {
				return;
			}

			const context = resizeCanvas(canvas);

			if (context === null) {
				return;
			}

			drawPredictiveCodingChart(
				context,
				canvas.clientWidth,
				canvas.clientHeight,
				history.current,
			);
		};

		const unregister = registerPainter("resonance", (updates) => {
			const frame = asFrames(updates).find(
				(row) => row?.symbol === focusSymbol,
			);

			if (frame === undefined) {
				return;
			}

			/*
				The batch republishes every retained carrier on each flush, so the
				same settled epoch arrives more than once. Appending it twice would
				stretch the series without any new observation behind it.
			*/
			if (history.current.at(-1)?.at === frame.at) {
				return;
			}

			history.current = [...history.current, frame].slice(-HISTORY);
			draw();
		});

		const observer = new ResizeObserver(draw);
		const canvas = canvasRef.current;

		if (canvas !== null) {
			observer.observe(canvas);
		}

		draw();

		return () => {
			unregister?.();
			observer.disconnect();
		};
	}, [focusSymbol]);

	return <canvas ref={canvasRef} className={className} />;
};

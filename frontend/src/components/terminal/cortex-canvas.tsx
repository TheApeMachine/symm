import { useEffect, useRef } from "react";
import {
	CortexLeafRoster,
	drawCortexTree,
} from "#/components/terminal/cortex-draw";
import { cortexTreeFromReading } from "#/components/terminal/cortex-tree";
import { registerPainter } from "#/providers/ws-stores";

/*
CortexCanvas draws the sensory prefix tree.

This is the one cognition reading that is not a value: it is a graph, and a
graph has to be laid out before it can be shown. data-paint writes a value into
a node, which is exactly the wrong shape for it, so the canvas registers its own
painter — the same arrangement the market graph already uses. What it draws is
still only what the classifier published: the branches, their probabilities, and
the beam path through them.

The leaf roster is retained across frames so a tree that gains a branch grows
into the space it already occupied rather than reshuffling every leaf.
*/
export const CortexCanvas = ({
	symbol,
	className,
}: {
	symbol: string;
	className?: string;
}) => {
	const canvasRef = useRef<HTMLCanvasElement>(null);
	const rosterRef = useRef(new CortexLeafRoster());
	const readingRef = useRef<Record<string, unknown> | null>(null);

	useEffect(() => {
		const draw = () => {
			const canvas = canvasRef.current;

			if (canvas === null) {
				return;
			}

			const width = Math.max(1, canvas.clientWidth);
			const height = Math.max(1, canvas.clientHeight);
			const ratio = window.devicePixelRatio || 1;

			if (
				canvas.width !== Math.floor(width * ratio) ||
				canvas.height !== Math.floor(height * ratio)
			) {
				canvas.width = Math.floor(width * ratio);
				canvas.height = Math.floor(height * ratio);
			}

			const context = canvas.getContext("2d");
			const tree = cortexTreeFromReading(readingRef.current);

			if (context === null) {
				return;
			}

			context.setTransform(ratio, 0, 0, ratio, 0, 0);

			if (tree === null) {
				context.clearRect(0, 0, width, height);
				return;
			}

			drawCortexTree(context, width, height, tree, rosterRef.current);
		};

		/*
			Cognition is published as a symbol-keyed map and only carries the symbols
			re-read this tick, so a frame that omits this one leaves the last tree on
			screen instead of blanking it.
		*/
		const unregister = registerPainter("cognition", (updates) => {
			if (updates === null || typeof updates !== "object") {
				return;
			}

			const reading = (updates as Record<string, unknown>)[symbol];

			if (reading === undefined || reading === null) {
				return;
			}

			readingRef.current = reading as Record<string, unknown>;
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
	}, [symbol]);

	return <canvas ref={canvasRef} className={className} />;
};

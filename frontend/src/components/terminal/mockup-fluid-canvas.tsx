import { type DependencyList, useEffect, useRef } from "react";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { resizeMockupCanvas } from "#/components/terminal/canvas";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";

type Watch = {
	store: string;
	key: string;
};

type CanvasDraw = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	buffers: Record<string, unknown[]>,
) => void;

/*
MockupFluidCanvas repaints with the tmp terminal canvas sizing contract.
*/
export const MockupFluidCanvas = ({
	draw,
	watches,
	deps,
	className,
}: {
	draw: CanvasDraw;
	watches: Watch[];
	deps?: DependencyList;
	className?: string;
}) => {
	const canvasRef = useRef<HTMLCanvasElement | null>(null);
	const drawRef = useRef(draw);
	const online = useSelector(appStore, (state) => state.online);

	drawRef.current = draw;

	useDirectStorePaint(
		getWorker(),
		watches,
		(buffers) => {
			const canvas = canvasRef.current;

			if (canvas === null) {
				return;
			}

			const context = resizeMockupCanvas(canvas);

			if (context === null) {
				return;
			}

			drawRef.current(
				context,
				canvas.clientWidth,
				canvas.clientHeight,
				buffers,
			);
		},
		[online, ...(deps ?? [])],
	);

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		const render = () => {
			const context = resizeMockupCanvas(canvas);

			if (context === null) {
				return;
			}

			drawRef.current(context, canvas.clientWidth, canvas.clientHeight, {});
		};

		const observer = new ResizeObserver(render);
		observer.observe(canvas);

		return () => observer.disconnect();
	}, []);

	return <canvas ref={canvasRef} className={className ?? "block size-full"} />;
};

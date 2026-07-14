import { type DependencyList, useEffect, useRef } from "react";
import { resizeMockupCanvas } from "#/components/terminal/canvas";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";

type StoreSubscription = {
	subscribe: (listener: () => void) => {
		unsubscribe: () => void;
	};
};

type CanvasDraw = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
) => void;

/*
MockupFluidCanvas repaints with the tmp terminal canvas sizing contract.
*/
export const MockupFluidCanvas = ({
	draw,
	stores,
	deps,
	className,
}: {
	draw: CanvasDraw;
	stores: StoreSubscription[];
	deps?: DependencyList;
	className?: string;
}) => {
	const canvasRef = useRef<HTMLCanvasElement | null>(null);
	const drawRef = useRef(draw);

	drawRef.current = draw;

	useDirectStorePaint(
		() => {
			const canvas = canvasRef.current;

			if (canvas === null) {
				return;
			}

			const context = resizeMockupCanvas(canvas);

			if (context === null) {
				return;
			}

			drawRef.current(context, canvas.clientWidth, canvas.clientHeight);
		},
		stores,
		deps ?? [],
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

			drawRef.current(context, canvas.clientWidth, canvas.clientHeight);
		};

		const observer = new ResizeObserver(render);
		observer.observe(canvas);

		return () => observer.disconnect();
	}, []);

	return <canvas ref={canvasRef} className={className ?? "block size-full"} />;
};

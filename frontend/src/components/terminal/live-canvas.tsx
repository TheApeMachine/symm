import { useEffect, useRef, type DependencyList } from "react";
import { resizeCanvas } from "#/components/terminal/canvas";
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
LiveCanvas renders a static canvas shell and repaints it from store subscriptions
on requestAnimationFrame, keeping chart updates off the React render path.
*/
export const LiveCanvas = ({
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

			const context = resizeCanvas(canvas);

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
			const context = resizeCanvas(canvas);

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

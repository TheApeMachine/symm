import { useEffect, useRef } from "react";
import {
	CortexLeafRoster,
	drawCortexTree,
} from "#/components/terminal/cortex-draw";
import { cortexTreeFromReading } from "#/components/terminal/cortex-tree";
import { hubBaseUrl } from "#/lib/hub";

/*
CortexCanvas draws the sensory prefix tree.
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
			if (canvas === null) return;

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

			if (context === null) return;

			context.setTransform(ratio, 0, 0, ratio, 0, 0);

			if (tree === null) {
				context.clearRect(0, 0, width, height);
				return;
			}

			drawCortexTree(context, width, height, tree, rosterRef.current);
		};

		const paint = (record: Record<string, unknown> | null): void => {
			readingRef.current = record;
			draw();
		};

		let cancelled = false;
		const loadTree = async () => {
			try {
				const response = await fetch(
					`${hubBaseUrl()}/cognition/tree?symbol=${encodeURIComponent(symbol)}`,
				);
				if (!response.ok) return;

				const treeData =
					(await response.json()) as Record<string, unknown> | null;
				if (!cancelled && treeData) {
					paint(treeData);
				}
			} catch {
				// hub may be connecting
			}
		};

		loadTree();
		const interval = setInterval(loadTree, 5000);

		const observer = new ResizeObserver(draw);
		const canvas = canvasRef.current;

		if (canvas !== null) {
			observer.observe(canvas);
		}

		draw();

		return () => {
			cancelled = true;
			clearInterval(interval);
			observer.disconnect();
		};
	}, [symbol]);

	return <canvas ref={canvasRef} className={className} />;
};

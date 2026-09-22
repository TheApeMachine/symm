import { type ComponentProps, useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import { resizeCanvas } from "./canvas-utils";
import type { Graph } from "./evidence-graph.types";
import {
	buildScene,
	drawEvidenceGraph,
	type GraphHit,
	type GraphScene,
	hitTest,
} from "./evidence-graph-viz";
import { GraphInspector } from "./graph-inspector";

export type EvidenceGraphProps = Omit<ComponentProps<"div">, "children"> & {
	graph?: Graph;
};

/* Graph data belongs to the producer. This component owns only layout and picking. */
export const EvidenceGraph = ({
	graph,
	className,
	...props
}: EvidenceGraphProps) => {
	const canvasRef = useRef<HTMLCanvasElement>(null);
	const sceneRef = useRef<GraphScene | null>(null);
	const [hover, setHover] = useState<{
		hit: GraphHit;
		x: number;
		y: number;
	} | null>(null);
	const hoverKey = hover?.hit.kind === "node" ? hover.hit.node.key : undefined;

	useEffect(() => setHover(null), [graph]);
	useEffect(() => {
		const canvas = canvasRef.current;

		if (!canvas) return;

		const draw = () => {
			const context = resizeCanvas(canvas);

			if (!context) return;

			const width = canvas.clientWidth;
			const height = canvas.clientHeight;
			const scene = graph ? buildScene(graph, width, height) : null;
			sceneRef.current = scene;
			drawEvidenceGraph(
				context,
				width,
				height,
				graph ?? null,
				scene ?? undefined,
				hoverKey,
			);
		};
		const observer = new ResizeObserver(draw);
		observer.observe(canvas);
		draw();
		return () => observer.disconnect();
	}, [graph, hoverKey]);

	return (
		<div
			className={cn(
				"relative min-h-64 overflow-hidden bg-(--sunken)",
				className,
			)}
			{...props}
		>
			<canvas
				ref={canvasRef}
				className="absolute inset-0 size-full"
				aria-label="Evidence graph"
				onMouseLeave={() => setHover(null)}
				onMouseMove={(event) => {
					if (!graph || !sceneRef.current) return;

					const bounds = event.currentTarget.getBoundingClientRect();
					const x = event.clientX - bounds.left;
					const y = event.clientY - bounds.top;
					const hit = hitTest(graph, sceneRef.current, x, y);
					setHover(hit ? { hit, x, y } : null);
				}}
			/>
			{hover && <GraphInspector {...hover} />}
		</div>
	);
};

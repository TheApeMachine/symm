import { createFileRoute } from "@tanstack/react-router";
import { createRef, useEffect } from "react";
import type { CognitiveReading } from "#/collections/types";
import { resizeCanvas } from "#/components/terminal/canvas";
import { drawCognitiveTree } from "#/components/terminal/cognitive-viz";
import { CortexLeafRoster } from "#/components/terminal/cortex-draw";
import {
	CortexBeamShell,
	CortexPanelsShell,
	paintCortexBeams,
	paintCortexPanels,
} from "#/components/terminal/cortex-panels";

const cortexCanvasRef = createRef<HTMLCanvasElement>();
const cortexCountRef = createRef<HTMLSpanElement>();
const cortexBeamRef = createRef<HTMLDivElement>();
const cortexPanelsRef = createRef<HTMLDivElement>();
const cortexRoster = new CortexLeafRoster();
let lastReading: CognitiveReading | null = null;

const drawCortexCanvas = (reading: CognitiveReading | null) => {
	const canvas = cortexCanvasRef.current;

	if (canvas === null) {
		return;
	}

	const context = resizeCanvas(canvas);

	if (context === null) {
		return;
	}

	drawCognitiveTree(
		context,
		canvas.clientWidth,
		canvas.clientHeight,
		reading as Record<string, unknown> | null,
		cortexRoster,
	);
};

/*
paintCortex draws the current DRAW batch of cognitive readings into the cortex
tree, beam shell, and side panels. Only this batch is used.
*/
export const paintCortex = (value: unknown, focusSymbol: string) => {
	const readings = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as CognitiveReading[];
	const reading =
		readings.find(
			(entry) => focusSymbol === "" || entry.symbol === focusSymbol,
		) ??
		readings.at(-1) ??
		null;

	lastReading = reading;
	drawCortexCanvas(reading);
	paintCortexBeams(
		cortexBeamRef.current,
		reading as Record<string, unknown> | null,
	);
	paintCortexPanels(
		cortexPanelsRef.current,
		reading as Record<string, unknown> | null,
	);

	if (cortexCountRef.current !== null) {
		cortexCountRef.current.textContent = `${readings.length} readings`;
	}
};

/*
CortexRoute is the static sensory-context shell. DRAW paints via paintCortex.
*/
const RouteComponent = () => {
	useEffect(() => {
		const canvas = cortexCanvasRef.current;

		if (canvas === null) {
			return;
		}

		const observer = new ResizeObserver(() => drawCortexCanvas(lastReading));
		observer.observe(canvas);

		return () => observer.disconnect();
	}, []);

	return (
		<div className="flex h-full min-w-[1140px] flex-col">
			<div className="flex h-[46px] shrink-0 items-center gap-2 overflow-x-auto border-(--line) border-b bg-(--surface) px-3.5">
				<span className="mr-1 shrink-0 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Sensory context
				</span>
				<span
					ref={cortexCountRef}
					className="ml-auto shrink-0 font-mono text-[10px] text-(--f4)"
				>
					0 readings
				</span>
			</div>
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_364px]">
				<div className="flex min-h-0 flex-col border-(--line) border-r">
					<div className="relative min-h-0 flex-[1.55] overflow-hidden bg-(--sunken)">
						<canvas
							ref={cortexCanvasRef}
							className="absolute inset-0 block h-full w-full bg-(--bg)"
						/>
						<div className="pointer-events-none absolute top-3 left-3.5">
							<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
								Sensory prefix tree · s/[sequence]
							</div>
							<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
								edge = P(next token | prefix) · amber = MAP beam path
							</div>
						</div>
						<div className="pointer-events-none absolute top-3 right-3.5 flex gap-[13px] font-mono text-[9px] text-(--f3)">
							<span className="inline-flex items-center gap-[5px]">
								<span className="h-[2px] w-2.5 bg-(--acc)" />
								beam
							</span>
							<span className="inline-flex items-center gap-[5px]">
								<span className="h-[2px] w-2.5 bg-(--line2)" />
								branch
							</span>
						</div>
					</div>

					<div className="flex min-h-0 flex-1 flex-col border-(--line) border-t bg-(--surface)">
						<div className="flex shrink-0 items-center justify-between border-(--line) border-b px-3 py-2">
							<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
								Beam search lookahead
							</span>
							<span className="font-mono text-[9.5px] text-(--f4)">
								log-prob
							</span>
						</div>
						<CortexBeamShell rootRef={cortexBeamRef} />
					</div>
				</div>

				<div className="min-h-0 overflow-auto bg-(--surface) p-3.5">
					<CortexPanelsShell rootRef={cortexPanelsRef} />
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/cortex")({
	component: RouteComponent,
});

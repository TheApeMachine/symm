import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { cognitiveStore } from "#/collections/cognitive";
import {
	CortexBeamList,
	CortexSidePanels,
} from "#/components/terminal/cortex-panels";
import { drawCognitiveTree } from "#/components/terminal/cognitive-viz";

const RouteComponent = () => {
	const readings = useSelector(cognitiveStore, (state) => state.readings);
	const canvasRef = useRef<HTMLCanvasElement | null>(null);

	// Lead with the crispest (least ambiguous, highest class confidence) symbol —
	// the regime the cognitive engine is most committed to this tick.
	const entries = Object.values(readings);
	const reading =
		entries
			.slice()
			.sort(
				(left, right) => right.classConfidence - left.classConfidence,
			)[0] ?? null;

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		const context = canvas.getContext("2d");

		if (context === null) {
			return;
		}

		drawCognitiveTree(
			context,
			canvas.width,
			canvas.height,
			reading as Record<string, unknown> | null,
		);
	}, [reading]);

	return (
		<div className="flex h-full min-w-[1140px] flex-col">
			<div className="flex shrink-0 items-center gap-[22px] border-(--line) border-b bg-(--surface) px-[18px] py-3">
				<div>
					<div className="font-serif font-semibold text-[18px] text-(--f1) leading-[1.1]">
						Cognitive tree
					</div>
					<div className="mt-[3px] font-mono text-[10px] text-(--f4)">
						sensory prefix tree · beam search lookahead · attractor basin
					</div>
				</div>
				{reading !== null ? (
					<span className="ml-auto font-mono text-[11px] text-(--f3)">
						{reading.scope} · {reading.winnerClass}
					</span>
				) : null}
			</div>

			{reading === null ? (
				<div className="flex flex-1 items-center justify-center font-mono text-[11px] text-(--f4)">
					waiting for cognitive frames
				</div>
			) : (
				<div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_360px]">
					<div className="flex min-h-0 flex-col">
						<canvas
							ref={canvasRef}
							width={760}
							height={420}
							className="w-full flex-1 bg-(--bg)"
						/>
						<div className="border-(--line) border-t">
							<CortexBeamList
								reading={reading as Record<string, unknown> | null}
							/>
						</div>
					</div>
					<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface)">
						<CortexSidePanels
							reading={reading as Record<string, unknown> | null}
						/>
					</div>
				</div>
			)}
		</div>
	);
};

export const Route = createFileRoute("/cortex")({
	component: RouteComponent,
});

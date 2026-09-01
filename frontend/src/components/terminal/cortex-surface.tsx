import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { cognitionStore, focusStore } from "#/collections/app";
import { CortexBeamShell } from "./cortex-beam-shell";
import { CortexCanvas } from "./cortex-canvas";
import { CortexPanelsShell } from "./cortex-panels-shell";

export const CortexSurface = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const root = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const apply = (state: typeof cognitionStore.state) => {
			if (!root.current) return;
			// The freshest frame for this symbol, from that symbol's own ring.
			const targetRow = state.getLast(focusSymbol) ?? null;

			const winner = root.current.querySelector<HTMLElement>("[data-winner]");
			const sequence =
				root.current.querySelector<HTMLElement>("[data-sequence]");

			if (winner) winner.textContent = targetRow?.winner() ?? "—";
			if (sequence) sequence.textContent = targetRow?.sequence() ?? "—";
		};

		apply(cognitionStore.state);
		const subscription = cognitionStore.subscribe(apply);
		return () => subscription.unsubscribe();
	}, [focusSymbol]);

	return (
		<div ref={root} className="flex h-full min-w-285 flex-col">
			<div className="flex h-11.5 shrink-0 items-center gap-2 overflow-x-auto border-(--line) border-b bg-(--surface) px-3.5">
				<span className="mr-1 shrink-0 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Sensory context
				</span>
				<span className="ml-auto shrink-0 font-mono text-[10px] text-(--f4)">
					<span data-winner /> · <span data-sequence />
				</span>
			</div>
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_364px]">
				<div className="flex min-h-0 flex-col border-(--line) border-r">
					<div className="relative min-h-0 flex-[1.55] overflow-hidden bg-(--sunken)">
						<CortexCanvas
							symbol={focusSymbol}
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
						<div className="pointer-events-none absolute top-3 right-3.5 flex gap-3.25 font-mono text-[9px] text-(--f3)">
							<span className="inline-flex items-center gap-1.25">
								<span className="h-0.5 w-2.5 bg-(--acc)" />
								beam
							</span>
							<span className="inline-flex items-center gap-1.25">
								<span className="h-0.5 w-2.5 bg-(--line2)" />
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
						<CortexBeamShell symbol={focusSymbol} />
					</div>
				</div>
				<div className="min-h-0 overflow-auto bg-(--surface) p-3.5">
					<CortexPanelsShell symbol={focusSymbol} />
				</div>
			</div>
		</div>
	);
};

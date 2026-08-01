import { Panel } from "@/components/ui/panel";
import { meterTrackVariants } from "@/components/ui/meter";
import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";

const BEAM_POOL = 8;

/*
CortexBeamShell renders a static shell for the cognitive beam list. Live beam
rows are painted into a fixed pool of pre-rendered rows by paintCortexBeams so
websocket cadence never re-renders this React tree.
*/
export const CortexBeamShell = () => (
	<Component registerKey="cortex-beams">
		{({ ref, className }) => (
			<div ref={ref} className={cn("flex min-h-0 flex-1 flex-col", className)}>
				<div
					data-cortex="waiting"
					className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)"
				>
					waiting for cognitive beam reading
				</div>

				<div
					data-cortex="content"
					style={{ display: "none" }}
					className="flex min-h-0 flex-1 flex-col gap-1.25 overflow-auto px-2 py-1.5"
				>
					{Array.from({ length: BEAM_POOL }, (_, index) => (
						<Panel
							// biome-ignore lint/suspicious/noArrayIndexKey: fixed pool
							key={index}
							size="s"
							className="flex items-center gap-2"
							data-cortex-beam={index}
							style={{ display: "none" }}
						>
							<span
								data-cortex="rank"
								className="w-4 shrink-0 font-mono text-[10px] text-(--info)"
							/>
							<span
								data-cortex="sequence"
								className="flex-1 font-mono text-[11px] text-(--f1)"
							/>
							<div
								data-cortex="meter-track"
								className={meterTrackVariants({ variant: "info", size: "xs" })}
								style={{ width: "70px" }}
							>
								<div
									data-cortex="meter-fill"
									className="h-full bg-(--meter-tone)"
									style={{ width: "0%" }}
								/>
							</div>
							<span
								data-cortex="score"
								className="w-11 shrink-0 text-right font-mono text-[9.5px] text-(--f3)"
							/>
						</Panel>
					))}
				</div>
			</div>
		)}
	</Component>
);

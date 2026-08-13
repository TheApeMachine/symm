import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";
import { meterTrackVariants } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import { Typography } from "@/components/ui/typography";

/*
CortexBeamShell lists the beam-search lookahead for the focused symbol.

Cognition publishes one row per symbol keyed by that symbol, so selecting the
focused row's predictions unrolls the list into one slot per projected path.
Each row shows the hops the search kept and the probability it assigned them.
*/
export const CortexBeamShell = ({ symbol }: { symbol: string }) => (
	<Component registerKey="cognition" select={`${symbol}.predictions`} repeat>
		{({ ref, className, slots }) => (
			<div ref={ref} className={cn("flex min-h-0 flex-1 flex-col", className)}>
				{slots.length === 0 ? (
					<div className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)">
						waiting for cognitive beam reading
					</div>
				) : (
					<div className="flex min-h-0 flex-1 flex-col gap-1.25 overflow-auto px-2 py-1.5">
						{slots.map((slot) => (
							<Panel
								key={slot}
								size="s"
								data-index={slot}
								className="flex items-center gap-2"
							>
								<span className="w-4 shrink-0 font-mono text-[10px] text-(--info)">
									{slot + 1}
								</span>
								<Typography.Span
									data-paint="predictedPath"
									className="flex-1 font-mono text-[11px] text-(--f1)"
								/>
								<div
									className={meterTrackVariants({
										variant: "info",
										size: "xs",
									})}
									style={{ width: "70px" }}
								>
									<div
										data-set="probability"
										data-target="style.--beam"
										className="h-full bg-(--meter-tone)"
										style={{ width: "calc(var(--beam, 0) * 100%)" }}
									/>
								</div>
								<Typography.Span
									data-paint="probability"
									data-paint-format=".1%"
									className="w-11 shrink-0 text-right font-mono text-[9.5px] text-(--f3)"
								/>
							</Panel>
						))}
					</div>
				)}
			</div>
		)}
	</Component>
);

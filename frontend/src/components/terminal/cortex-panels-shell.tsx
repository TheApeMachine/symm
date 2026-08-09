import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";
import { Badge } from "@/components/ui/badge";
import { meterTrackVariants } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import { Stat } from "@/components/ui/stat";
import { Typography } from "@/components/ui/typography";

/*
CortexPanelsShell is the cognition side rail.

Cognition publishes one row per symbol, so the rail scopes itself to the
focused one and paints that row's fields directly. Every panel here shows a
reading the classifier actually produces: which regime won, how sure it was,
how surprising the sequence was, and whether the branch was decisive.
*/
export const CortexPanelsShell = ({ symbol }: { symbol: string }) => (
	<Component registerKey="cognition" select={symbol}>
		{({ ref, className }) => (
			<div ref={ref} className={cn("flex flex-col gap-3.5", className)}>
				<Panel>
					<div className="flex items-center justify-between">
						<span className="font-semibold text-[12px] text-(--f1)">
							Attractor basin · classify
						</span>
						<Badge label="classify" variant="warning" />
					</div>
					<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
						softmax posterior · b/[class]/[sequence]
					</div>
					<div className="flex flex-col gap-2">
						<div className="flex items-center justify-between font-mono text-[10px]">
							<Typography.Span
								data-paint="winner"
								className="text-(--f3)"
							/>
							<Typography.Span
								data-paint="confidence"
								data-paint-format=".1%"
								className="text-(--f1)"
							/>
						</div>
						<div className={meterTrackVariants({ variant: "info", size: "m" })}>
							<div
								data-set="confidence"
								data-target="style.--basin"
								className="h-full bg-(--meter-tone)"
								style={{ width: "calc(var(--basin, 0) * 100%)" }}
							/>
						</div>
					</div>
				</Panel>

				<Panel>
					<div className="font-semibold text-[12px] text-(--f1)">
						Contrastive evidence
					</div>
					<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
						routing margin · winner vs runner-up
					</div>
					<div className="grid grid-cols-2 gap-2.5 text-center">
						<Stat
							layout="feature"
							label="contrast"
							value={
								<Typography.Span
									data-paint="contrast"
									data-paint-format=".3f"
								/>
							}
							variant="warning"
						/>
						<Stat
							layout="feature"
							label="entropy bits"
							value={
								<Typography.Span
									data-paint="entropyBits"
									data-paint-format=".3f"
								/>
							}
							variant="warning"
						/>
					</div>
				</Panel>

				<Panel>
					<div className="flex items-center justify-between">
						<span className="font-semibold text-[12px] text-(--f1)">
							Branch entropy gate
						</span>
						{/*
							A branch is decisive until the classifier reports it
							ambiguous, so the badge is driven by that flag rather
							than by a threshold restated here.
						*/}
						<Typography.Span
							data-paint="ambiguous"
							data-paint-class="true:text-(--warn) false:text-(--up)"
							className="font-semibold text-[9px] uppercase tracking-wide"
						/>
					</div>
					<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
						shannon H vs uniform threshold
					</div>
					<div>
						<div
							className={meterTrackVariants({ variant: "success", size: "m" })}
						>
							<div
								data-set="entropyBits"
								data-target="style.--entropy"
								className="h-full bg-(--meter-tone)"
								style={{ width: "min(100%, calc(var(--entropy, 0) * 25%))" }}
							/>
						</div>
					</div>
				</Panel>

				<Panel>
					<div className="flex items-center justify-between">
						<span className="font-semibold text-[12px] text-(--f1)">
							REM consolidation
						</span>
						{/*
							data-paint-prop diverts the boolean into the dataset instead
							of textContent, leaving the two words below for CSS to pick
							from and their own tone classes intact.
						*/}
						<span
							data-paint="remConsolidating"
							data-paint-prop="dataset.consolidating"
							className="group rounded-full border border-(--line2) px-2.25 py-px font-mono text-[9px]"
						>
							<span className="text-(--f3) group-data-[consolidating=true]:hidden">
								awake
							</span>
							<span className="hidden text-(--acc) group-data-[consolidating=true]:inline">
								REM
							</span>
						</span>
					</div>
					<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
						episodic replay · decay · retroactive inhibition
					</div>
					<div className="grid grid-cols-3 gap-2">
						<Stat
							layout="feature"
							label="decay γ"
							value={
								<Typography.Span
									data-paint="remDecayFactor"
									data-paint-format=".3f"
								/>
							}
						/>
						<Stat
							layout="feature"
							label="replays"
							value={<Typography.Span data-paint="remReplays" />}
						/>
						<Stat
							layout="feature"
							label="inhibition"
							value={
								<Typography.Span
									data-paint="remInhibitionPct"
									data-paint-format=".0%"
								/>
							}
						/>
					</div>
				</Panel>
			</div>
		)}
	</Component>
);

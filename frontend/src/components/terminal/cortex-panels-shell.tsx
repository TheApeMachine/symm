import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";
import { Badge } from "@/components/ui/badge";
import { meterTrackVariants } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import { Stat } from "@/components/ui/stat";
import { Component } from "#/components/ui/component";

const CLASS_POOL = 6;

/*
CortexPanelsShell renders the static chrome for the four cortex side panels.
Live values are painted into data-cortex hooks by paintCortexPanels.
*/
export const CortexPanelsShell = () => (
	<Component register={(paint) => registerPainter("cortex-panels", paint)}>
		{({ ref, className }) => (
	<div ref={ref} className={cn("flex flex-col gap-3.5", className)}>
		<Panel>
			<div className="flex items-center justify-between">
				<span className="font-semibold text-[12px] text-(--f1)">
					Attractor basin · classify
				</span>
				<Badge label="waiting 0%" variant="warning" data-cortex="basin-badge" />
			</div>
			<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
				softmax posterior · b/[class]/[sequence]
			</div>
			<div className="flex flex-col gap-2">
				<div
					data-cortex="basin-waiting"
					className="font-mono text-[10px] text-(--f4)"
				>
					waiting for attractor basin
				</div>
				{Array.from({ length: CLASS_POOL }, (_, index) => (
					<div
						// biome-ignore lint/suspicious/noArrayIndexKey: fixed pool
						key={index}
						data-cortex-class={index}
						className="flex items-center gap-2"
						style={{ display: "none" }}
					>
						<span
							data-cortex="class-name"
							className="w-16 font-mono text-[10px] text-(--f3)"
						/>
						<div
							data-cortex="meter-track"
							className={meterTrackVariants({ variant: "info", size: "m" })}
							style={{ flex: "1 1 0%" }}
						>
							<div
								data-cortex="meter-fill"
								className="h-full bg-(--meter-tone)"
								style={{ width: "0%" }}
							/>
						</div>
						<span
							data-cortex="class-percent"
							className="w-8 text-right font-mono text-[10px] text-(--f2)"
						/>
					</div>
				))}
			</div>
		</Panel>

		<Panel>
			<div className="font-semibold text-[12px] text-(--f1)">
				Contrastive evidence
			</div>
			<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
				routing margin · winner vs runner-up
			</div>
			<div className="grid grid-cols-3 gap-2.5 text-center">
				<Stat
					layout="feature"
					label="winner bits"
					value=""
					variant="success"
					data-cortex="winner-bits"
				/>
				<Stat
					layout="feature"
					label="runner-up bits"
					value=""
					variant="warning"
					data-cortex="runner-bits"
				/>
				<Stat
					layout="feature"
					label="KL divergence"
					value=""
					variant="warning"
					data-cortex="kl"
				/>
			</div>
			<div className="mt-3">
				<div className="mb-1 flex justify-between font-mono text-[9.5px] text-(--f4)">
					<span>separation margin</span>
					<span data-cortex="margin-value" />
				</div>
				<div
					data-cortex="margin-track"
					className={meterTrackVariants({ variant: "warning", size: "s" })}
				>
					<div
						data-cortex="margin-fill"
						className="h-full bg-(--meter-tone)"
						style={{ width: "0%" }}
					/>
				</div>
			</div>
		</Panel>

		<Panel>
			<div className="flex items-center justify-between">
				<span className="font-semibold text-[12px] text-(--f1)">
					Branch entropy gate
				</span>
				<Badge label="decisive" variant="success" data-cortex="entropy-badge" />
			</div>
			<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
				shannon H vs uniform threshold
			</div>
			<div>
				<div className="mb-1 flex justify-between font-mono text-[10px]">
					<span data-cortex="entropy-label" className="text-(--f3)" />
					<span data-cortex="entropy-value" className="text-(--f1)" />
				</div>
				<div
					data-cortex="entropy-track"
					className={meterTrackVariants({ variant: "success", size: "m" })}
				>
					<div
						data-cortex="entropy-fill"
						className="h-full bg-(--meter-tone)"
						style={{ width: "0%" }}
					/>
				</div>
			</div>
		</Panel>

		<Panel>
			<div className="flex items-center justify-between">
				<span className="font-semibold text-[12px] text-(--f1)">
					REM consolidation
				</span>
				<Badge label="waiting" variant="warning" data-cortex="rem-badge" />
			</div>
			<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
				episodic replay · decay · retroactive inhibition
			</div>
			<div className="grid grid-cols-3 gap-2 font-mono">
				<Stat layout="tile" label="window" value="" data-cortex="rem-window" />
				<Stat layout="tile" label="replays" value="" data-cortex="rem-replays" />
				<Stat layout="tile" label="cohort" value="" data-cortex="rem-cohort" />
			</div>
		</Panel>
	</div>
		)}
	</Component>
);

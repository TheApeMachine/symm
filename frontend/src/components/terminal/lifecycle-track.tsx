import { cn } from "#/lib/utils";
import type { LifecycleState } from "#/types/thesis";
import {
	LIFECYCLE_STAGES,
	LIFECYCLE_TERMINAL,
	lifecycleStageIndex,
} from "#/types/thesis";
import { Badge } from "@/components/ui/badge";

const stageLabel = (stage: string): string => stage.replaceAll("_", " ");

const stageTone = (stage: LifecycleState, current: LifecycleState): string => {
	if (stage === current) {
		return "bg-[color-mix(in_srgb,var(--acc)_28%,transparent)] text-(--acc) border-[color-mix(in_srgb,var(--acc)_40%,transparent)]";
	}

	const currentIndex = lifecycleStageIndex(current);
	const stageIndex = lifecycleStageIndex(stage);

	if (currentIndex < 0 || stageIndex < 0) {
		return "bg-(--line) text-(--f4) border-transparent";
	}

	if (stageIndex < currentIndex) {
		return "bg-[color-mix(in_srgb,var(--up)_14%,transparent)] text-(--up) border-transparent";
	}

	return "bg-(--sunken) text-(--f4) border-transparent";
};

/*
LifecycleTrack renders one symbol's backend lifecycle state against the ordered
stage rail so progress is visible without inferring it from execution frames.
*/
export const LifecycleTrack = ({
	symbol,
	state,
}: {
	symbol: string;
	state: LifecycleState;
}) => {
	const currentIndex = lifecycleStageIndex(state);
	const terminal = LIFECYCLE_TERMINAL.has(state);

	return (
		<div className="rounded border border-(--line) bg-(--surface) px-3 py-2.5">
			<div className="mb-2 flex items-center justify-between gap-2">
				<span className="font-mono font-semibold text-[12px] text-(--f1)">
					{symbol}
				</span>
				<Badge
					label={stageLabel(state)}
					variant={
						terminal
							? state === "evaluated"
								? "success"
								: "error"
							: currentIndex >= 6 && currentIndex <= 10
								? "warning"
								: "info"
					}
					size="xs"
				/>
			</div>
			<div className="flex flex-wrap gap-1">
				{LIFECYCLE_STAGES.map((stage) => (
					<span
						key={stage}
						title={stageLabel(stage)}
						className={cn(
							"rounded-[2px] border px-1 py-px font-mono text-[8px] uppercase tracking-wide",
							stageTone(stage, state),
						)}
					>
						{stage.split("_")[0]}
					</span>
				))}
			</div>
		</div>
	);
};

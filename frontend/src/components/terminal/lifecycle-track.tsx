import { cn } from "#/lib/utils";
import type { LifecycleState } from "#/types/thesis";
import {
	LIFECYCLE_MANAGING,
	LIFECYCLE_STAGES,
	LIFECYCLE_TERMINAL,
	lifecycleStageIndex,
} from "#/types/thesis";
import { badgeVariants } from "@/components/ui/badge";

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

const badgeVariantFor = (
	state: LifecycleState,
): "success" | "error" | "warning" | "info" => {
	if (LIFECYCLE_TERMINAL.has(state)) {
		if (state === "evaluated") {
			return "success";
		}

		return "error";
	}

	if (LIFECYCLE_MANAGING.has(state)) {
		return "warning";
	}

	return "info";
};

/*
paintLifecycleTrack writes the live lifecycle badge and stage rail into a mounted
LifecycleTrack shell so journal ticks never re-render the React tree.
*/
export const paintLifecycleTrack = (
	root: HTMLElement | null,
	state: LifecycleState,
): void => {
	if (root === null) {
		return;
	}

	const badge = root.querySelector("[data-lifecycle='badge']");

	if (badge instanceof HTMLElement) {
		badge.textContent = stageLabel(state);
		badge.className = cn(
			badgeVariants({ variant: badgeVariantFor(state), size: "xs" }),
		);
	}

	for (const stage of LIFECYCLE_STAGES) {
		const node = root.querySelector(`[data-lifecycle-stage='${stage}']`);

		if (!(node instanceof HTMLElement)) {
			continue;
		}

		node.className = cn(
			"rounded-[2px] border px-1 py-px font-mono text-[8px] uppercase tracking-wide",
			stageTone(stage, state),
		);
	}
};

/*
LifecycleTrack renders one symbol's backend lifecycle state against the ordered
stage rail so progress is visible without inferring it from execution frames.
*/
export const LifecycleTrack = ({
	symbol,
	state = "observing",
}: {
	symbol: string;
	state?: LifecycleState;
}) => (
	<div
		data-lifecycle-track={symbol}
		className="rounded border border-(--line) bg-(--surface) px-3 py-2.5"
	>
		<div className="mb-2 flex items-center justify-between gap-2">
			<span
				data-lifecycle="symbol"
				className="font-mono font-semibold text-[12px] text-(--f1)"
			>
				{symbol}
			</span>
			<span
				data-lifecycle="badge"
				className={cn(
					badgeVariants({ variant: badgeVariantFor(state), size: "xs" }),
				)}
			>
				{stageLabel(state)}
			</span>
		</div>
		<div className="flex flex-wrap gap-1">
			{LIFECYCLE_STAGES.map((stage) => (
				<span
					key={stage}
					data-lifecycle-stage={stage}
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

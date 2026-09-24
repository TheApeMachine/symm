import type { ComponentProps } from "react";
import { EpisodeTape, type TrainingEpisode } from "./episode-tape";
import { Flex } from "./flex";
import { type OutcomeBin, OutcomeDistribution } from "./outcome-distribution";
import { type PolicyBranch, PolicyBranches } from "./policy-branches";
import { Stat } from "./stat";

export interface ForwardSummary {
	/** Measured fractional return, converted to basis points for display. */
	edge?: number;
	decisions?: number | string;
	accuracy?: number;
}
export type ForwardViewProps = Omit<
	ComponentProps<typeof Flex.Column>,
	"children"
> & {
	episode?: TrainingEpisode | null;
	branches?: PolicyBranch[] | null;
	outcomeBins?: OutcomeBin[] | null;
	summary?: ForwardSummary | null;
};

/* Shared composition for the existing learning page and graph-authored pages. */
export const ForwardView = ({
	episode,
	branches,
	outcomeBins,
	summary,
	...props
}: ForwardViewProps) => (
	<Flex.Column {...props}>
		<Flex.Row className="min-h-0 flex-1 flex-wrap border-b border-(--line)">
			<EpisodeTape
				episode={episode}
				className="min-w-64 flex-1 bg-(--sunken) p-4"
			/>
			<Flex.Column className="gap-4 p-4">
				<Stat
					label="Mean decision benefit"
					value={
						summary?.edge === undefined
							? "—"
							: `${(summary?.edge * 10000).toFixed(1)} bp`
					}
				/>
				<Stat
					label="Learned situations"
					value={
						summary?.decisions === undefined
							? "—"
							: summary?.decisions.toLocaleString()
					}
				/>
				<Stat
					label="Accuracy"
					value={
						summary?.accuracy === undefined
							? "—"
							: `${(summary?.accuracy * 100).toFixed(1)}%`
					}
				/>
			</Flex.Column>
		</Flex.Row>
		<Flex.Row className="min-h-0 flex-1 flex-wrap">
			<PolicyBranches branches={branches} className="flex-1 min-w-64 p-4" />
			<OutcomeDistribution
				bins={outcomeBins}
				unit="bp"
				className="flex-1 min-w-64 p-4 bg-(--sunken)"
			/>
		</Flex.Row>
	</Flex.Column>
);

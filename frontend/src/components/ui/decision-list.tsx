import { cn } from "@/lib/utils";
import { Flex } from "./flex";
import { List } from "./list";
import { Typography } from "./typography";

/*
DecisionRow is one decision as a reader sees it: which instrument, what was
decided, how sure the system was, and what it said about why.
*/
export type DecisionRow = {
	id: string;
	symbol: string;
	action: string;
	confidence: number;
	reason: string;
};

export type DecisionListProps = {
	className?: string;
	title?: string;
	decisions?: DecisionRow[];
	onInspect?: (symbol: string) => void;
	emptyLabel?: string;
};

/*
DecisionList draws what the system decided, one row per decision it is handed;
id tells the rows apart, so one instrument may stand in several.

It reaches for nothing: the rows and what a click means arrive as props, so the
same list serves a React surface reading the strategy stream and a
graph-authored one handed the same rows.
*/
export const DecisionList = ({
	className,
	title = "DECISIONS",
	decisions = [],
	onInspect,
	emptyLabel = "waiting for backend decision frames",
}: DecisionListProps) => (
	<Flex.Column className={cn("h-full min-h-0 gap-0", className)}>
		<Flex.Row
			align="baseline"
			justify="between"
			padding={2}
			className="border-(--line) border-b"
		>
			<Typography.Span semibold uppercase tracking="0.13em">
				{title}
			</Typography.Span>
		</Flex.Row>
		<List className="min-h-0 flex-1 gap-1 overflow-auto p-2">
			{decisions.length === 0 ? (
				<List.Item className="grid cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-0 px-2.5 py-1.5 font-mono text-[11px] text-(--f4)">
					{emptyLabel}
				</List.Item>
			) : (
				decisions.map((decision) => (
					<List.Item
						key={decision.id}
						className="grid cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-0 rounded-[3px] border border-(--line) bg-(--sunken) px-2.5 py-1.5 transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)] hover:bg-(--raised)"
						data-decision-card="true"
						data-decision-id={decision.id}
						onClick={() => onInspect?.(decision.symbol)}
						title="Inspect MCTS / Pearl decision tree"
					>
						<Typography.Span className="truncate font-semibold text-[11px] text-(--f1)">
							{decision.symbol}
						</Typography.Span>
						<Flex.Row className="items-center gap-2">
							<Typography.Span className="text-[8.5px] text-(--f4)">
								conf=
								<span className="tabular-nums text-(--f2)">
									{decision.confidence.toFixed(4)}
								</span>
							</Typography.Span>
							<Typography.Span className="rounded-xs border border-(--line) px-1.5 py-px text-[8.5px] uppercase">
								{decision.action}
							</Typography.Span>
						</Flex.Row>
						<Typography.Span className="col-span-2 mt-0.5 min-w-0 truncate text-[9px] leading-tight text-(--f4)">
							<span title={decision.reason}>{decision.reason}</span>
						</Typography.Span>
					</List.Item>
				))
			)}
		</List>
	</Flex.Column>
);

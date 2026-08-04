import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { Flex } from "@/components/ui/flex";

const slotCount = 6;
const decisionSlotKeys = Array.from(
	{ length: slotCount },
	(_, index) => `decision-slot-${index + 1}`,
);

export const Decisions = () => (
	<Component registerKey="decisions">
		{({ ref, className }) => (
			<Flex.Column ref={ref} className={cn("h-full min-h-0 gap-0", className)}>
				<Flex.Row
					align="baseline"
					justify="between"
					padding={2}
					className="border-(--line) border-b"
				>
					<Typography.Span semibold uppercase tracking="0.13em">
						DECISIONS
					</Typography.Span>
				</Flex.Row>
				<Flex.Row className="grid grid-cols-[minmax(0,1fr)_8ch_6.5rem] gap-3 px-3 py-1.25 text-[9.5px] uppercase tracking-[0.06em] text-(--f4)">
					<Typography.Span>Symbol</Typography.Span>
					<Typography.Span className="text-right">Utility</Typography.Span>
					<Typography.Span className="text-right">Action</Typography.Span>
				</Flex.Row>
				<List className="min-h-0 flex-1 overflow-auto">
					{decisionSlotKeys.map((slotKey, index) => (
						<List.Item
							key={slotKey}
							className="grid grid-cols-[minmax(0,1fr)_8ch_6.5rem] gap-3 px-3"
							data-index={index}
						>
							<Typography.Span
								data-paint="symbol"
								className="truncate text-(--f1)"
							/>
							<Typography.Span
								data-paint="utility"
								data-paint-format=".4f"
								className="shrink-0 text-right tabular-nums text-(--f2)"
							/>
							<Typography.Span
								data-paint="action"
								className="shrink-0 text-right text-(--acc) uppercase"
							/>
						</List.Item>
					))}
				</List>
			</Flex.Column>
		)}
	</Component>
);

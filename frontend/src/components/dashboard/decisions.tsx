import type { StrategyDecision } from "#/collections/types";
import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";
import { Flex } from "@/components/ui/flex";

const slotCount = 6;

type RowModel = {
	visible: string;
	symbol: string;
	utility: string;
	action: string;
};

type PaintModel = {
	summary: string;
	rows: RowModel[];
};

const normalizeDecisions = (value: unknown): StrategyDecision[] => {
	if (Array.isArray(value)) return value as StrategyDecision[];
	if (value !== null && typeof value === "object")
		return Object.values(value as Record<string, StrategyDecision>);
	return value != null ? [value as StrategyDecision] : [];
};

const hiddenRow = (): RowModel => ({
	visible: "none",
	symbol: "",
	utility: "",
	action: "",
});

const buildModel = (rows: StrategyDecision[]): PaintModel => {
	const sorted = rows
		.slice()
		.sort((left, right) => left.symbol.localeCompare(right.symbol));
	const active = sorted.filter(
		(row) => row.action.toLowerCase() !== "nothing",
	).length;
	const items = Array.from({ length: slotCount }, () => hiddenRow());

	sorted.slice(0, slotCount).forEach((row, index) => {
		items[index] = {
			visible: "",
			symbol: row.symbol,
			utility: Number.isFinite(row.utility) ? row.utility.toFixed(4) : "—",
			action: row.action,
		};
	});

	return {
		summary: `${active} active · ${sorted.length - active} passive`,
		rows: items,
	};
};

export const Decisions = () => (
	<Component
		register={(paint) =>
			registerPainter("decisions", (updates) => {
				paint(buildModel(normalizeDecisions(updates)));
			})
		}
		select="rows"
	>
		{({ ref, className, slots }) => (
			<Flex.Column ref={ref} className="gap-2">
				<Flex.Row className="items-baseline justify-between border-(--line) border-b px-1 pb-2">
					<Typography.Span className="text-[11px] tracking-[0.22em] text-(--f3)">
						DECISIONS
					</Typography.Span>
					<Typography.Span
						data-paint="summary"
						className="text-[10px] text-(--f4)"
					/>
				</Flex.Row>
				<Flex.Row className="justify-between px-1 text-[9px] uppercase tracking-[0.18em] text-(--f4)">
					<Typography.Span>Symbol</Typography.Span>
					<Typography.Span>Utility</Typography.Span>
					<Typography.Span>Action</Typography.Span>
				</Flex.Row>
				<List
					className={cn("min-h-0 flex-1 border-(--line) border-b", className)}
				>
					{slots.map((index) => (
						<List.Item
							key={`${index}-decision`}
							className="justify-between"
							data-select="rows"
							data-index={index}
							data-paint="visible"
							data-paint-prop="style.display"
						>
							<Typography.Span
								data-paint="symbol"
								className="truncate text-(--f1)"
							/>
							<Typography.Span
								data-paint="utility"
								className="shrink-0 text-(--f2) text-right"
							/>
							<Typography.Span
								data-paint="action"
								className="shrink-0 text-(--acc) uppercase"
							/>
						</List.Item>
					))}
				</List>
			</Flex.Column>
		)}
	</Component>
);

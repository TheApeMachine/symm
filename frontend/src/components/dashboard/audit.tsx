import { Component } from "#/components/ui/component";
import type { Position } from "#/collections/types";
import { holdingAuditRow } from "#/components/terminal/dashboard-audit";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";

const buildRows = (value: unknown) => {
	const rows = (Array.isArray(value)
		? value
		: value !== null && typeof value === "object"
			? Object.values(value as Record<string, Position>)
			: value != null
				? [value]
				: []) as Position[];

	return {
		rows: rows
			.map((row) => row.holding)
			.filter(Boolean)
			.map((holding) => holdingAuditRow(holding)),
	};
};

export const AuditTrail = () => (
	<Component
		register={(paint) =>
			registerPainter("positions", (updates) => {
				paint(buildRows(updates));
			})
		}
		select="rows"
	>
		{({ ref, className, slots }) => (
			<List
				ref={ref}
				className={cn("min-h-0 flex-1 border-(--line) border-b", className)}
			>
				{slots?.map((slot) => (
					<List.Item
						key={slot}
						className="justify-between"
						data-index={slot}
					>
						<Typography.Span data-paint="reason" />
						<Typography.Span data-paint="reference" />
						<Typography.Span data-paint="meta" />
					</List.Item>
				))}
			</List>
		)}
	</Component>
);

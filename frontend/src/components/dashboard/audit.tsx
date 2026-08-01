import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";

export const AuditTrail = () => (
	<Component
		registerKey="positions"
		select="holding"
	>
		{({ ref, className, slots }) => (
			<List
				ref={ref}
				className={cn("min-h-0 flex-1 border-(--line) border-b", className)}
			>
				{slots.map((slot) => (
					<List.Item
						key={slot}
						className="justify-between"
						data-index={slot}
					>
						<Typography.Span data-paint="status" />
						<Typography.Span data-paint="symbol" />
						<Typography.Span>
							pnl <span data-paint="pnl" data-paint-format=".4f" /> · return{" "}
							<span data-paint="return_pct" data-paint-format=".4f" />%
						</Typography.Span>
					</List.Item>
				))}
			</List>
		)}
	</Component>
);

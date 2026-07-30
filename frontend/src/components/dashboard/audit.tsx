import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";

export const AuditTrail = () => (
	<Component register={(paint) => registerPainter("audit", paint)}>
		{({ ref, className, slots }) => (
			<List
				ref={ref}
				className={cn("min-h-0 flex-1 border-(--line) border-b", className)}
			>
				{slots?.map((_slot, idx) => (
					<List.Item key={idx} className="justify-between">
						<Typography.Span data-paint="" />
						<Typography.Span data-paint="" />
						<Typography.Span data-paint="" />
					</List.Item>
				))}
			</List>
		)}
	</Component>
);

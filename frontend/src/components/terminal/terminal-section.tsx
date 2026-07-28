import type { ReactNode } from "react";
import { cn } from "#/lib/utils";

export const TerminalSection = ({
	title,
	meta,
	children,
	className,
}: {
	title: string;
	meta?: ReactNode;
	children: ReactNode;
	className?: string;
}) => (
	<div
		className={cn(
			"flex min-h-0 flex-col overflow-hidden bg-(--surface)",
			className,
		)}
	>
		<div className="flex shrink-0 items-center justify-between border-(--line) border-b px-3 py-2">
			<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
				{title}
			</span>
			{meta ? (
				<span className="font-mono text-[10px] text-(--f4)">{meta}</span>
			) : null}
		</div>
		{children}
	</div>
);

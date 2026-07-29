import type { ReactNode } from "react";
import { cn } from "#/lib/utils";

export const ColumnHeader = ({
	title,
	meta,
	ref,
	className,
}: {
	title: string;
	meta?: ReactNode;
	ref?: React.Ref<HTMLDivElement>;
	className?: string;
}) => (
	<div
		ref={ref}
		className={cn(
			"sticky top-0 z-2 flex items-center justify-between border-(--line) border-b bg-(--surface) px-3 py-2",
			className,
		)}
	>
		<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
			{title}
		</span>
		{meta ? (
			<span className="font-mono text-[10px] text-(--f4)">{meta}</span>
		) : null}
	</div>
);

import type { HTMLAttributes, ReactNode } from "react";
import { cn } from "#/lib/utils";

export type ReadoutProps = HTMLAttributes<HTMLDivElement> & {
	label: string;
	value?: ReactNode;
	dataKey?: string;
	dot?: boolean;
	tone?: string;
	meta?: ReactNode;
	children?: ReactNode;
};

export const Readout = ({
	label,
	value = "—",
	dataKey,
	dot = false,
	tone = "text-(--f1)",
	meta,
	children,
	className,
	...props
}: ReadoutProps) => {
	const keyAttr = dataKey ? { "data-k": dataKey } : {};

	return (
		<div
			className={cn(
				"flex flex-col justify-between gap-1 rounded-sm border border-(--line) bg-[#0a0907] px-2.5 py-2",
				className,
			)}
			{...props}
		>
			<div className="flex items-center justify-between">
				<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					{label}
				</span>
				{meta ? (
					<span className="font-mono text-[8px] text-(--f4)">{meta}</span>
				) : null}
			</div>
			<div className="flex items-baseline gap-2">
				{dot ? (
					<span
						data-k={dataKey ? `${dataKey}-dot` : "dot"}
						className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)"
					/>
				) : null}
				<span
					{...keyAttr}
					className={cn(
						"truncate font-mono text-[12px] font-bold tabular-nums",
						tone,
					)}
				>
					{value}
				</span>
			</div>
			{children}
		</div>
	);
};

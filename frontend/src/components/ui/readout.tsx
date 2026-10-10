import type { ComponentProps, ReactNode } from "react";
import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";

export type ReadoutProps = Omit<
	ComponentProps<typeof Flex.Column>,
	"children"
> & {
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
		<Flex.Column
			justify="between"
			gap={1}
			className={cn(
				"rounded-sm border border-(--line) bg-[#0a0907] px-2.5 py-2",
				className,
			)}
			{...props}
		>
			<Flex.Row align="center" justify="between">
				<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					{label}
				</span>
				{meta ? (
					<span className="font-mono text-[8px] text-(--f4)">{meta}</span>
				) : null}
			</Flex.Row>
			<Flex.Row align="baseline" gap={2}>
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
			</Flex.Row>
			{children}
		</Flex.Column>
	);
};

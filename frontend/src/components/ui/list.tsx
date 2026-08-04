import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Flex } from "./flex";

/*
List is a stack of rows. Item is a row you read; Option is a row you choose.

The distinction is worth keeping. An Item carries information and takes hover
only so the eye can track across it. An Option is a control: it renders a
<button>, it owns a selected state that keyboard navigation drives, and its
label and hint sit on a fixed two-line grid so a column of them scans evenly
instead of reading as a pile of differently-shaped rows.
*/

export type ListProps = ComponentProps<typeof Flex.Column>;

export const List = ({ className, children, ...props }: ListProps) => (
	<Flex.Column gap={1} className={cn("min-h-0", className)} {...props}>
		{children}
	</Flex.Column>
);

export const listItemVariants = cva(
	"rounded-[3px] px-2 py-1 text-[11px] font-medium text-(--f1)",
	{
		variants: {
			interactive: {
				true: "cursor-pointer transition-colors hover:bg-(--sunken)",
				false: "",
			},
		},
		defaultVariants: {
			interactive: true,
		},
	},
);

export type ListItemProps = ComponentProps<typeof Flex.Row> &
	VariantProps<typeof listItemVariants>;

List.Item = ({ className, interactive, children, ...props }: ListItemProps) => (
	<Flex.Row
		gap={2}
		align="center"
		{...props}
		className={cn(listItemVariants({ interactive }), className)}
	>
		{children}
	</Flex.Row>
);

export const listOptionVariants = cva(
	[
		"flex w-full cursor-pointer items-center gap-2.5 rounded-[4px]",
		"border border-transparent bg-transparent text-left",
		"hover:bg-(--raised)",
	],
	{
		variants: {
			size: {
				s: "px-2.5 py-1.5",
				m: "px-3 py-2.5",
			},
			/*
				Selected is where the keyboard cursor is; it moves on every arrow key.
				Active is what the surface is currently showing. They are different
				facts and a list often shows both at once, so they stay separate props
				rather than collapsing into one three-state enum.
			*/
			selected: {
				true: "border-(--acc) bg-[color-mix(in_srgb,var(--acc)_10%,transparent)]",
			},
		},
		defaultVariants: {
			size: "m",
		},
	},
);

type ListOptionVariantProps = VariantProps<typeof listOptionVariants>;

export type ListOptionProps = Omit<ComponentProps<"button">, "children"> &
	ListOptionVariantProps & {
		icon?: ReactNode;
		label: ReactNode;
		hint?: ReactNode;
		trailing?: ReactNode;
		/* True when this row is the one the surface is currently showing. */
		active?: boolean;
	};

List.Option = ({
	ref,
	icon,
	label,
	hint,
	trailing,
	size,
	selected,
	active,
	type = "button",
	className,
	...props
}: ListOptionProps) => (
	<button
		ref={ref}
		type={type}
		aria-current={active ? "true" : undefined}
		className={cn(listOptionVariants({ size, selected }), className)}
		{...props}
	>
		{icon}
		<div className="min-w-0 flex-1">
			<div
				className={cn(
					"truncate font-medium text-[13px]",
					active ? "text-(--acc)" : "text-(--f1)",
				)}
			>
				{label}
			</div>
			{hint === undefined ? null : (
				<div className="truncate font-mono text-[10px] text-(--f4)">{hint}</div>
			)}
		</div>
		{trailing}
	</button>
);

export type ListEmptyProps = ComponentProps<"div">;

/*
An empty state is copy, not decoration: the caller passes the sentence, because
"no match for what you typed" and "the engine has published nothing yet" are
different facts and only the caller knows which one applies.
*/
List.Empty = ({ className, children, ...props }: ListEmptyProps) => (
	<div
		className={cn(
			"px-3.5 py-6.5 text-center font-mono text-[12px] text-(--f4)",
			className,
		)}
		{...props}
	>
		{children}
	</div>
);

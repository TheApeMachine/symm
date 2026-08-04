import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ElementType, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Typography } from "./typography";

/*
Nav is the surface rail: labelled groups of destinations down the left edge,
with whatever the app wants pinned to the bottom.

Item is polymorphic on purpose. A rail entry is a link in a routed app, a button
in a tabbed one, and an <a> in a static page — and a component library that
hard-wired one of those would be unusable in the other two. Pass the element via
`as` and every prop it needs rides along untouched:

	<Nav.Item as={Link} to="/graph" icon={<Icon name="graph" />} label="Graph" />

Active is passed in rather than derived. The rail does not know what routing
looks like, and guessing at it is how a sidebar ends up disagreeing with the
page it is sitting next to.
*/

export const navVariants = cva("flex shrink-0 flex-col", {
	variants: {
		surface: {
			surface: "bg-(--surface)",
			sunken: "bg-(--sunken)",
			none: "",
		},
		border: {
			right: "border-(--line) border-r",
			left: "border-(--line) border-l",
			none: "",
		},
	},
	defaultVariants: {
		surface: "surface",
		border: "right",
	},
});

export type NavProps = ComponentProps<"nav"> & VariantProps<typeof navVariants>;

export const Nav = ({
	ref,
	surface,
	border,
	className,
	children,
	...props
}: NavProps) => (
	<nav
		ref={ref}
		className={cn(navVariants({ surface, border }), className)}
		{...props}
	>
		{children}
	</nav>
);

export type NavGroupProps = ComponentProps<"div"> & {
	label?: ReactNode;
};

/*
The gap between groups belongs to the group, not to its label. Putting it on the
label meant a not-first test against the label's own parent, where the label is
always first — so it never matched and every group hugged the one above it. On
the group itself the test is against its sibling groups, which is the
relationship actually being described.
*/
Nav.Group = ({ ref, label, className, children, ...props }: NavGroupProps) => (
	<div
		ref={ref}
		className={cn("flex flex-col not-first:mt-1.5", className)}
		{...props}
	>
		{label === undefined ? null : (
			<Typography.Label size="s" tone="f4" className="px-2.5 pt-3 pb-1.5">
				{label}
			</Typography.Label>
		)}
		<div className="flex flex-col gap-0.75 px-2">{children}</div>
	</div>
);

export const navItemVariants = cva(
	[
		"flex cursor-pointer items-center gap-2 rounded-[3px] border px-2.25 py-2",
		"text-left text-[13px] font-medium",
		"hover:bg-(--raised)",
	],
	{
		variants: {
			active: {
				true: "border-(--line2) bg-(--raised) text-(--f1)",
				false: "border-transparent bg-transparent text-(--f3)",
			},
		},
		defaultVariants: {
			active: false,
		},
	},
);

export type NavItemProps<T extends ElementType = "button"> = {
	as?: T;
	icon?: ReactNode;
	label: ReactNode;
	active?: boolean;
	className?: string;
} & Omit<ComponentProps<T>, "as" | "children" | "className">;

Nav.Item = <T extends ElementType = "button">({
	as,
	icon,
	label,
	active,
	className,
	...props
}: NavItemProps<T>) => {
	const Element = (as ?? "button") as ElementType;

	return (
		<Element
			className={cn(navItemVariants({ active }), className)}
			aria-current={active ? "page" : undefined}
			{...(Element === "button" ? { type: "button" } : {})}
			{...props}
		>
			{icon}
			{label}
		</Element>
	);
};

/*
Footer sits at the bottom of the rail whatever the group list does, which is
what `mt-auto` buys — no spacer element, no fixed rail height.
*/
Nav.Footer = ({
	ref,
	className,
	children,
	...props
}: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn(
			"mt-auto border-(--line) border-t p-2.5 font-mono text-[10px] text-(--f4) leading-[1.6]",
			className,
		)}
		{...props}
	>
		{children}
	</div>
);

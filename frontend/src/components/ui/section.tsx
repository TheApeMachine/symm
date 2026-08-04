import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Typography } from "./typography";

/*
Section is a titled region of a surface: a rule-separated header strip with an
overline on the left and a quiet mono readout on the right, over a body that
scrolls on its own.

Three near-identical versions of this header existed before it had a name — one
sticky, one not, one inlined into a route — and they disagreed about padding and
about which foreground step the title sits on. They are one thing now, and the
differences that were real became variants: `sticky` for a header that must stay
put over a scrolling column, `bare` for one already inside a bordered panel.
*/

export const sectionVariants = cva("flex flex-col", {
	variants: {
		surface: {
			none: "",
			surface: "bg-(--surface)",
			sunken: "bg-(--sunken)",
			raised: "bg-(--raised)",
		},
		fit: {
			/* Owns its slice of a flex column and scrolls its body inside it. */
			pane: "min-h-0 overflow-hidden",
			/* Grows to its content; the scroll belongs to an ancestor. */
			content: "",
		},
	},
	defaultVariants: {
		surface: "surface",
		fit: "pane",
	},
});

export const sectionHeaderVariants = cva("flex shrink-0 items-center gap-3", {
	variants: {
		size: {
			s: "px-2.5 py-1.5",
			m: "px-3 py-2",
			lg: "h-11.5 px-3.5",
			/* No box of its own: for a header sitting inside padded content. */
			bare: "mb-3 items-baseline",
		},
		/*
			The rule belongs to the header, not to the body under it. A header that
			drew no rule while its neighbour drew one is how a column ends up with
			doubled and missing separators in the same view.
		*/
		rule: {
			true: "border-(--line) border-b",
			false: "",
		},
		sticky: {
			true: "sticky top-0 z-2 bg-(--surface)",
		},
	},
	compoundVariants: [{ size: "bare", rule: true, class: "pb-1.5" }],
	defaultVariants: {
		size: "m",
		rule: true,
	},
});

type SectionVariantProps = VariantProps<typeof sectionVariants>;

export type SectionProps = ComponentProps<"div"> & SectionVariantProps;

export const Section = ({
	ref,
	surface,
	fit,
	className,
	children,
	...props
}: SectionProps) => (
	<div
		ref={ref}
		className={cn(sectionVariants({ surface, fit }), className)}
		{...props}
	>
		{children}
	</div>
);

type SectionHeaderVariantProps = VariantProps<typeof sectionHeaderVariants>;

export type SectionHeaderProps = Omit<ComponentProps<"div">, "title"> &
	SectionHeaderVariantProps & {
		/*
			Optional: a header strip that is only controls — a search field and its
			filters — is still a header, and giving it an empty title would put an
			empty overline in the layout.
		*/
		title?: ReactNode;
		/*
			Meta is the right-hand readout — a count, a timestamp, a symbol. It takes
			a node so a painted <span data-paint> can live there and the header never
			has to re-render to stay current.
		*/
		meta?: ReactNode;
		/* Controls that sit between title and meta. */
		children?: ReactNode;
	};

Section.Header = ({
	ref,
	title,
	meta,
	size,
	rule,
	sticky,
	className,
	children,
	...props
}: SectionHeaderProps) => (
	<div
		ref={ref}
		className={cn(
			sectionHeaderVariants({ size, rule, sticky }),
			children === undefined && meta !== undefined ? "justify-between" : null,
			className,
		)}
		{...props}
	>
		{title === undefined ? null : (
			<Typography.Label size="m" tone="f3" className="shrink-0">
				{title}
			</Typography.Label>
		)}
		{children}
		{meta === undefined ? null : (
			<Typography.Mono size="s" tone="f4" className="ml-auto shrink-0">
				{meta}
			</Typography.Mono>
		)}
	</div>
);

export type SectionBodyProps = ComponentProps<"div"> & {
	scroll?: boolean;
};

Section.Body = ({
	ref,
	scroll = true,
	className,
	children,
	...props
}: SectionBodyProps) => (
	<div
		ref={ref}
		className={cn(
			"min-h-0 flex-1",
			scroll ? "overflow-auto" : "overflow-hidden",
			className,
		)}
		{...props}
	>
		{children}
	</div>
);

import { cva, type VariantProps } from "class-variance-authority";
import { type HTMLMotionProps, motion } from "motion/react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export const typographyVariants = cva("font-mono text-[11.5px] text-(--f3)", {
	variants: {
		variant: {
			foreground: "text-foreground",
			/** Muted helpers under a page title */
			lead: "font-mono text-[11.5px] text-muted-foreground [&]:leading-normal max-sm:[&]:text-sm",
			/** Compact section label in side rails / panels */
			sectionHeading: "text-sm font-medium text-foreground",
			/** Monospace export / raw source preview */
			codeExport:
				"font-mono font-normal text-[10px] leading-relaxed whitespace-pre-wrap text-foreground sm:text-xs",
			info: "text-info",
			success: "text-success",
			warning: "text-warning",
			error: "text-error",
			muted: "text-muted-foreground",
			primary: "text-primary-foreground",
			secondary: "text-secondary-foreground",
		},
	},
	defaultVariants: {
		variant: "foreground",
	},
});

export type TypographyVariant = NonNullable<
	VariantProps<typeof typographyVariants>["variant"]
>;

type TypographyTextProps = {
	children?: ReactNode;
	variant?: TypographyVariant;
	truncate?: boolean;
	className?: string;
	semibold?: boolean;
	uppercase?: boolean;
	tracking?: string;
	leading?: string;
	normal?: boolean;
	light?: boolean;
	bold?: boolean;
	italic?: boolean;
	underline?: boolean;
};

export const Typography = ({ children }: { children: ReactNode }) => {
	return children;
};

Typography.PageTitle = ({
	children,
	className,
	...props
}: {
	children: ReactNode;
	className?: string;
	props?: unknown;
}) => {
	return (
		<h1
			className={cn("font-semibold text-foreground text-lg", className)}
			{...props}
		>
			{children}
		</h1>
	);
};

Typography.Title = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<h1
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</h1>
	);
};

Typography.Subtitle = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<h2
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</h2>
	);
};

Typography.H3 = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<h3
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</h3>
	);
};

Typography.H4 = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<h4
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</h4>
	);
};

Typography.H5 = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<h5
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</h5>
	);
};

Typography.H6 = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<h6
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</h6>
	);
};

Typography.Paragraph = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<p
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</p>
	);
};

Typography.Small = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<small
			className={cn(
				typographyVariants({ variant }),
				"text-xs leading-normal font-normal",
				truncate && "block min-w-0 w-full truncate",
				className,
			)}
		>
			{children}
		</small>
	);
};

Typography.Span = ({
	children,
	variant,
	truncate,
	className,
	semibold,
	uppercase,
	tracking,
	leading,
	normal,
	light,
	bold,
	italic,
	underline,
	...props
}: HTMLMotionProps<"span"> & TypographyTextProps) => {
	return (
		<motion.span
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
				semibold && "font-semibold",
				uppercase && "uppercase",
				tracking && `tracking-[${tracking}]`,
				leading && `leading-[${leading}]`,
				normal && "font-normal",
				light && "font-light",
				bold && "font-bold",
				italic && "italic",
				underline && "underline",
			)}
			{...props}
		>
			{children}
		</motion.span>
	);
};

Typography.Blockquote = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<blockquote
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</blockquote>
	);
};

Typography.Code = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<code
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</code>
	);
};

Typography.Pre = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<pre
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</pre>
	);
};

Typography.Kbd = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<kbd
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</kbd>
	);
};

Typography.Mark = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<mark
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</mark>
	);
};

Typography.S = ({
	children,
	variant,
	truncate,
	className,
}: TypographyTextProps) => {
	return (
		<s
			className={cn(
				typographyVariants({ variant }),
				truncate && "truncate",
				className,
			)}
		>
			{children}
		</s>
	);
};

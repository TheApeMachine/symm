import { cva, type VariantProps } from "class-variance-authority";
import { type HTMLMotionProps, motion } from "motion/react";
import type { ComponentProps, CSSProperties, ElementType, JSX } from "react";
import { cn } from "@/lib/utils";

/*
Typography is the text layer, and every element in it forwards its props.

That last part is the whole point. A painted surface drops a bare
<span data-paint="metrics.strength.raw" data-paint-format=".2f" /> into the tree
and lets the websocket write its textContent — no re-render, no state. A text
primitive that swallowed data-* attributes would quietly make itself unusable
for exactly the surfaces this library exists to build, so none of them do.

The same rule covers ref: a painted region is found by querySelector inside the
ref its Component handed down, so anything that might sit inside one must be
able to receive a ref.
*/

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
			/* Terminal foreground ramp, loudest to quietest. */
			f1: "text-(--f1)",
			f2: "text-(--f2)",
			f3: "text-(--f3)",
			f4: "text-(--f4)",
			accent: "text-(--acc)",
		},
	},
	defaultVariants: {
		variant: "foreground",
	},
});

export type TypographyVariant = NonNullable<
	VariantProps<typeof typographyVariants>["variant"]
>;

/*
Weight and decoration are booleans rather than a `weight` scale because that is
how they are reached for at call sites: one flag, on or off, beside the text.
*/
type TextModifiers = {
	variant?: TypographyVariant;
	truncate?: boolean;
	semibold?: boolean;
	uppercase?: boolean;
	normal?: boolean;
	light?: boolean;
	bold?: boolean;
	italic?: boolean;
	underline?: boolean;
	/*
		Tracking and leading are inline styles, not classes. An arbitrary Tailwind
		value assembled at runtime is never seen by the build's class scan, so
		`tracking-[${value}]` produces no CSS at all — the style property does.
	*/
	tracking?: string;
	leading?: string;
};

const modifierClass = (modifiers: TextModifiers): string =>
	cn(
		typographyVariants({ variant: modifiers.variant }),
		modifiers.truncate && "truncate",
		modifiers.semibold && "font-semibold",
		modifiers.uppercase && "uppercase",
		modifiers.normal && "font-normal",
		modifiers.light && "font-light",
		modifiers.bold && "font-bold",
		modifiers.italic && "italic",
		modifiers.underline && "underline",
	);

const modifierStyle = (
	modifiers: TextModifiers,
	style: CSSProperties | undefined,
): CSSProperties | undefined => {
	if (modifiers.tracking === undefined && modifiers.leading === undefined) {
		return style;
	}

	return {
		...style,
		...(modifiers.tracking === undefined
			? {}
			: { letterSpacing: modifiers.tracking }),
		...(modifiers.leading === undefined
			? {}
			: { lineHeight: modifiers.leading }),
	};
};

type TextProps<T extends ElementType> = Omit<ComponentProps<T>, "color"> &
	TextModifiers;

/*
textElement builds one tag's component. Splitting the modifiers off and spreading
everything that remains is what keeps data-* and ref intact.
*/
const textElement = <T extends keyof JSX.IntrinsicElements>(
	Tag: T,
	base?: string,
) => {
	const Text = ({
		variant,
		truncate,
		semibold,
		uppercase,
		normal,
		light,
		bold,
		italic,
		underline,
		tracking,
		leading,
		className,
		style,
		...props
	}: TextProps<T>) => {
		const modifiers: TextModifiers = {
			variant,
			truncate,
			semibold,
			uppercase,
			normal,
			light,
			bold,
			italic,
			underline,
			tracking,
			leading,
		};

		const Element = Tag as ElementType;

		return (
			<Element
				className={cn(base, modifierClass(modifiers), className)}
				style={modifierStyle(modifiers, style as CSSProperties | undefined)}
				{...props}
			/>
		);
	};

	Text.displayName = `Typography.${String(Tag)}`;

	return Text;
};

export const Typography = ({ children }: { children: React.ReactNode }) =>
	children;

Typography.PageTitle = textElement(
	"h1",
	"font-semibold text-foreground text-lg",
);
Typography.Title = textElement("h1");
Typography.Subtitle = textElement("h2");
Typography.H3 = textElement("h3");
Typography.H4 = textElement("h4");
Typography.H5 = textElement("h5");
Typography.H6 = textElement("h6");
Typography.Paragraph = textElement("p");
Typography.Small = textElement("small", "text-xs leading-normal font-normal");
Typography.Blockquote = textElement("blockquote");
Typography.Code = textElement("code");
Typography.Pre = textElement("pre");
Typography.Kbd = textElement("kbd");
Typography.Mark = textElement("mark");
Typography.S = textElement("s");
Typography.Div = textElement("div");

/*
Label is the overline that titles a rail, a column, or a stat — small, tracked
out, and upper case. It appeared, hand-rolled and slightly different every time,
in more than fifty places before it had a name; the sizes below are the spread
those hand-rolled versions actually used, collapsed to a scale.
*/
export const labelVariants = cva("uppercase", {
	variants: {
		size: {
			xxs: "text-[8px] tracking-[0.06em]",
			xs: "text-[8.5px] tracking-[0.08em]",
			s: "text-[9px] tracking-[0.14em]",
			m: "text-[10px] tracking-[0.13em]",
			lg: "text-[11px] tracking-[0.12em]",
			xl: "text-xs tracking-[0.1em]",
			xxl: "text-sm tracking-[0.08em]",
		},
		tone: {
			f1: "text-(--f1)",
			f2: "text-(--f2)",
			f3: "text-(--f3)",
			f4: "text-(--f4)",
			accent: "text-(--acc)",
		},
		weight: {
			normal: "font-normal",
			medium: "font-medium",
			semibold: "font-semibold",
		},
	},
	defaultVariants: {
		size: "m",
		tone: "f3",
		weight: "semibold",
	},
});

export type LabelProps = ComponentProps<"span"> &
	VariantProps<typeof labelVariants>;

Typography.Label = ({
	ref,
	size,
	tone,
	weight,
	className,
	...props
}: LabelProps) => (
	<span
		ref={ref}
		className={cn(labelVariants({ size, tone, weight }), className)}
		{...props}
	/>
);

/*
Span stays a motion element: it is the one text node call sites animate, and
losing that would mean reaching past the library for every fade-in.
*/
Typography.Span = ({
	variant,
	truncate,
	semibold,
	uppercase,
	normal,
	light,
	bold,
	italic,
	underline,
	tracking,
	leading,
	className,
	style,
	...props
}: HTMLMotionProps<"span"> & TextModifiers) => {
	const modifiers: TextModifiers = {
		variant,
		truncate,
		semibold,
		uppercase,
		normal,
		light,
		bold,
		italic,
		underline,
		tracking,
		leading,
	};

	return (
		<motion.span
			className={cn(modifierClass(modifiers), className)}
			style={modifierStyle(modifiers, style as CSSProperties | undefined)}
			{...props}
		/>
	);
};

/*
Mono is the numeric readout: a fixed-advance figure that will not reflow its
neighbours when a digit changes, which matters when the value is being written
several times a second.
*/
export const monoVariants = cva("font-mono tabular-nums", {
	variants: {
		size: {
			xxs: "text-[8.5px]",
			xs: "text-[9.5px]",
			s: "text-[10px]",
			m: "text-[11px]",
			lg: "text-[12px]",
			xl: "text-[13px]",
			xxl: "text-sm",
		},
		tone: {
			f1: "text-(--f1)",
			f2: "text-(--f2)",
			f3: "text-(--f3)",
			f4: "text-(--f4)",
			accent: "text-(--acc)",
			up: "text-(--up)",
			down: "text-(--down)",
		},
		weight: {
			normal: "font-normal",
			medium: "font-medium",
			semibold: "font-semibold",
		},
	},
	defaultVariants: {
		size: "m",
		tone: "f2",
		weight: "normal",
	},
});

export type MonoProps = ComponentProps<"span"> &
	VariantProps<typeof monoVariants>;

Typography.Mono = ({
	ref,
	size,
	tone,
	weight,
	className,
	...props
}: MonoProps) => (
	<span
		ref={ref}
		className={cn(monoVariants({ size, tone, weight }), className)}
		{...props}
	/>
);

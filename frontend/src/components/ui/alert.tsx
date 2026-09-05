import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Icon, type IconName } from "./icon";

/*
Alert is a full-width band that says the surface below it is not telling the
whole truth: a failed fetch, a stale feed, a degraded mode.

It is tinted from the same recipe as Badge — a 40% border and a 12% wash of the
tone, with the text at full strength — so a failure reads as the same colour of
event wherever it appears. That mattering is the reason this exists: the three
error strips in this app were each hand-rolled, and one of them was rendering in
plain foreground, so an error looked like a caption.

Unlike a Badge, it owns its row. A Badge labels a thing inside a layout; an
Alert interrupts the layout, which is why it draws a rule and spans the width.
*/

export const alertVariants = cva(
	[
		"flex w-full shrink-0 items-start gap-2 border-b px-3 py-2",
		"[--alert-tone:var(--error)]",
		"border-[color:color-mix(in_srgb,var(--alert-tone)_40%,transparent)]",
		"bg-[color:color-mix(in_srgb,var(--alert-tone)_12%,transparent)]",
		"font-mono text-[color:var(--alert-tone)]",
	],
	{
		variants: {
			variant: {
				info: "[--alert-tone:var(--info)]",
				success: "[--alert-tone:var(--success)]",
				warning: "[--alert-tone:var(--warning)]",
				error: "[--alert-tone:var(--error)]",
			},
			size: {
				s: "text-[10px]",
				m: "text-[11px]",
				lg: "text-xs",
			},
			/*
				The rule belongs to the band by default, for the same reason it belongs
				to Section.Header: a strip that drew none while its neighbours did is
				how a column ends up with a gap where a line should be. Turn it off
				when the band is the last thing in its container.
			*/
			rule: {
				true: "",
				false: "border-b-0",
			},
		},
		defaultVariants: {
			variant: "error",
			size: "m",
			rule: true,
		},
	},
);

export type AlertProps = Omit<ComponentProps<"div">, "title"> &
	VariantProps<typeof alertVariants> & {
		/*
			The leading glyph. A failure and a warning carry `broken` — this set's
			signal trace, cut — because a band that only changes colour is a band a
			colour-blind reader cannot tell from a notice. Pass a name to override,
			or `false` for a bare band.
		*/
		icon?: IconName | false;
		children?: ReactNode;
	};

/*
Only the two bands that report something wrong carry a glyph by default. An info
or success band is already unambiguous from its text, and marking those too
would make the glyph mean "this is an Alert" rather than "this is a failure".
*/
const DEFAULT_ICON: Partial<Record<string, IconName>> = {
	error: "broken",
	warning: "broken",
};

export const Alert = ({
	ref,
	variant,
	size,
	rule,
	icon,
	className,
	children,
	...props
}: AlertProps) => {
	const name =
		icon === false ? undefined : (icon ?? DEFAULT_ICON[variant ?? "error"]);

	return (
		<div
			ref={ref}
			role="alert"
			className={cn(alertVariants({ variant, size, rule }), className)}
			{...props}
		>
			{name === undefined ? null : (
				<Icon name={name} size="s" className="mt-px shrink-0" />
			)}
			<span className="min-w-0">{children}</span>
		</div>
	);
};

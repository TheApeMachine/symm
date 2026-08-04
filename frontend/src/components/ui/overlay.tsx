import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";

/*
Overlay is the scrim plus the box that centres what sits on it. Modal and the
command palette are both this; they differ only in where the content lands and
how wide it is.

Two details are easy to get wrong and are therefore fixed here. First, the
dismiss target is a real <button> stretched across the scrim rather than a click
handler on the backdrop div — a keyboard user gets a focusable way out, and the
handler cannot fire on clicks that bubble up from the content. Second, the
content wrapper is pointer-events-none while the content itself re-enables them,
so the gap beside a narrow panel still dismisses.

`open` is honoured through the `hidden` attribute rather than by returning null,
so a surface painted through querySelector keeps its nodes in the tree across
open and close and never has to re-bind them.
*/

export const overlayVariants = cva(
	[
		"absolute inset-0 z-9 flex flex-col p-8 backdrop-blur-[3px]",
		"motion-safe:animate-[symFade_.16s_ease]",
	],
	{
		variants: {
			variant: {
				dim: "bg-[color-mix(in_srgb,var(--sunken)_60%,transparent)]",
				solid: "bg-(--sunken)",
				heavy: "bg-[color-mix(in_srgb,var(--sunken)_64%,transparent)]",
			},
		},
		defaultVariants: {
			variant: "dim",
		},
	},
);

export const overlayContentVariants = cva(
	"pointer-events-none relative z-10 flex min-h-0 flex-1 justify-center",
	{
		variants: {
			align: {
				center: "items-center",
				/* A palette belongs under the eye line, not in the middle. */
				top: "items-start pt-24",
			},
		},
		defaultVariants: {
			align: "center",
		},
	},
);

type OverlayVariantProps = VariantProps<typeof overlayVariants> &
	VariantProps<typeof overlayContentVariants>;

export type OverlayProps = Omit<ComponentProps<"div">, "children"> &
	OverlayVariantProps & {
		open?: boolean;
		onClose?: () => void;
		/* Names the dismiss control for assistive tech. */
		closeLabel?: string;
		children: ReactNode;
	};

export const Overlay = ({
	ref,
	open,
	onClose,
	closeLabel = "Close",
	variant,
	align,
	className,
	children,
	...props
}: OverlayProps) => (
	<div
		ref={ref}
		className={cn(overlayVariants({ variant }), className)}
		{...props}
		{...(open === undefined ? {} : { hidden: !open })}
	>
		{onClose === undefined ? null : (
			<button
				type="button"
				aria-label={closeLabel}
				className="absolute inset-0 cursor-default"
				onClick={onClose}
			/>
		)}
		<div className={overlayContentVariants({ align })}>{children}</div>
	</div>
);

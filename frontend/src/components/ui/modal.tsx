import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Button } from "./button";
import { Icon } from "./icon";
import { Overlay, overlayVariants } from "./overlay";

/*
Modal is an Overlay carrying a surface panel. Compose chrome with Modal.Header,
Modal.Body, Modal.Footer, and Modal.Close.

The scrim is Overlay's, re-exported here under its old name so callers that
style the backdrop keep working.
*/
export const modalScrimVariants = overlayVariants;

export const modalPanelVariants = cva(
	[
		"pointer-events-auto flex max-h-full w-full flex-col overflow-hidden",
		"border border-(--line2) bg-(--surface)",
		"shadow-[0_22px_54px_-14px_rgba(0,0,0,0.72)]",
	],
	{
		variants: {
			size: {
				s: "max-w-[360px] rounded-[4px]",
				m: "max-w-[452px] rounded-[6px]",
				lg: "max-w-[560px] rounded-[6px]",
				/* Wide enough for a two-line result row plus its trailing tag. */
				xl: "max-w-[680px] rounded-lg",
			},
		},
		defaultVariants: {
			size: "m",
		},
	},
);

type ModalPanelVariantProps = VariantProps<typeof modalPanelVariants>;

export type ModalProps = Omit<ComponentProps<typeof Overlay>, "children"> &
	ModalPanelVariantProps & {
		children: ReactNode;
		/* Styles the panel; `className` still styles the scrim. */
		panelClassName?: string;
	};

export const Modal = ({
	ref,
	open,
	onClose,
	variant,
	align,
	size,
	className,
	panelClassName,
	children,
	...props
}: ModalProps) => (
	<Overlay
		ref={ref}
		open={open}
		onClose={onClose}
		variant={variant}
		align={align}
		className={className}
		{...props}
	>
		<div className={cn(modalPanelVariants({ size }), panelClassName)}>
			{children}
		</div>
	</Overlay>
);

Modal.Header = ({
	ref,
	className,
	children,
	...props
}: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn(
			"flex shrink-0 items-start justify-between gap-2.5 border-(--line) border-b px-4 pt-3.5 pb-[13px]",
			className,
		)}
		{...props}
	>
		{children}
	</div>
);

Modal.Body = ({
	ref,
	className,
	children,
	...props
}: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn(
			"min-h-0 flex-1 overflow-y-auto px-4 pt-3.5 pb-0.5",
			className,
		)}
		{...props}
	>
		{children}
	</div>
);

Modal.Footer = ({
	ref,
	className,
	children,
	...props
}: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn(
			"mt-[11px] flex shrink-0 items-center justify-between gap-3 border-(--line) border-t bg-(--sunken) px-4 py-3.5",
			className,
		)}
		{...props}
	>
		{children}
	</div>
);

Modal.Close = ({
	ref,
	className,
	"aria-label": ariaLabel = "Close",
	...props
}: ComponentProps<"button">) => (
	<Button
		ref={ref}
		variant="outline"
		size="s"
		shape="icon"
		aria-label={ariaLabel}
		className={className}
		{...props}
	>
		<Icon name="close" size="s" />
	</Button>
);

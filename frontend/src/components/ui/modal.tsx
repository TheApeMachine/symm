import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";

export const modalScrimVariants = cva(
	"absolute inset-0 z-9 flex flex-col p-8 backdrop-blur-[3px]",
	{
		variants: {
			variant: {
				dim: "bg-[color-mix(in_srgb,var(--sunken)_60%,transparent)]",
				solid: "bg-(--sunken)",
			},
		},
		defaultVariants: {
			variant: "dim",
		},
	},
);

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
			},
		},
		defaultVariants: {
			size: "m",
		},
	},
);

type ModalScrimVariantProps = VariantProps<typeof modalScrimVariants>;
type ModalPanelVariantProps = VariantProps<typeof modalPanelVariants>;

export type ModalProps = Omit<ComponentProps<"div">, "children"> &
	ModalScrimVariantProps &
	ModalPanelVariantProps & {
		open?: boolean;
		onClose?: () => void;
		children: ReactNode;
	};

/*
Modal is a dismissible overlay shell with a centered surface panel. Compose
chrome with Modal.Header, Modal.Body, Modal.Footer, and Modal.Close.
*/
export const Modal = ({
	ref,
	open,
	onClose,
	variant,
	size,
	className,
	children,
	...props
}: ModalProps) => (
	<div
		ref={ref}
		className={cn(modalScrimVariants({ variant }), className)}
		{...props}
		{...(open === undefined ? {} : { hidden: !open })}
	>
		{onClose === undefined ? null : (
			<button
				type="button"
				aria-label="Close"
				className="absolute inset-0"
				onClick={onClose}
			/>
		)}
		<div className="pointer-events-none relative z-10 flex min-h-0 flex-1 items-center justify-center">
			<div className={modalPanelVariants({ size })}>{children}</div>
		</div>
	</div>
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
	type = "button",
	...props
}: ComponentProps<"button">) => (
	<button
		ref={ref}
		type={type}
		className={cn(
			"flex size-[25px] shrink-0 cursor-pointer items-center justify-center rounded-[3px] border border-(--line) bg-(--raised) p-0 text-(--f3) hover:border-(--line2) hover:text-(--f1)",
			className,
		)}
		{...props}
	>
		<svg
			width="13"
			height="13"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			strokeWidth="2"
			aria-hidden="true"
		>
			<path d="M6 6l12 12M18 6L6 18" />
		</svg>
	</button>
);

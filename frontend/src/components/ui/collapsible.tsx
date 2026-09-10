"use client";

import {
	type ComponentProps,
	createContext,
	useContext,
	useId,
	useState,
} from "react";
import { cn } from "@/lib/utils";

/*
Collapsible is a disclosure: a trigger, and a panel it shows or hides.

It is uncontrolled by default and controllable by prop, because both callers
exist — a node's optional detail remembers nothing, while a rail's section is
driven by state that outlives it.

The open state is published as `data-panel-open` on both parts. Styling a
disclosure means reacting to it from the trigger's own subtree (rotating a
chevron, say), and a data attribute is the only handle a caller's Tailwind can
reach that from.
*/

type CollapsibleState = {
	open: boolean;
	toggle: () => void;
	panelId: string;
};

const CollapsibleContext = createContext<CollapsibleState | null>(null);

const useCollapsible = (part: string): CollapsibleState => {
	const state = useContext(CollapsibleContext);

	if (!state) {
		throw new Error(`${part} must be rendered inside a Collapsible`);
	}

	return state;
};

export type CollapsibleProps = Omit<ComponentProps<"div">, "onToggle"> & {
	defaultOpen?: boolean;
	open?: boolean;
	onOpenChange?: (open: boolean) => void;
};

export const Collapsible = ({
	ref,
	defaultOpen = false,
	open,
	onOpenChange,
	className,
	children,
	...props
}: CollapsibleProps) => {
	const [uncontrolled, setUncontrolled] = useState(defaultOpen);
	const panelId = useId();
	const isOpen = open ?? uncontrolled;

	const toggle = () => {
		const next = !isOpen;

		if (open === undefined) {
			setUncontrolled(next);
		}

		onOpenChange?.(next);
	};

	return (
		<CollapsibleContext.Provider value={{ open: isOpen, toggle, panelId }}>
			<div
				ref={ref}
				className={cn("flex min-w-0 flex-col", className)}
				{...props}
			>
				{children}
			</div>
		</CollapsibleContext.Provider>
	);
};

export const CollapsibleTrigger = ({
	ref,
	className,
	onClick,
	...props
}: ComponentProps<"button">) => {
	const { open, toggle, panelId } = useCollapsible("CollapsibleTrigger");

	return (
		<button
			ref={ref}
			type="button"
			aria-expanded={open}
			aria-controls={panelId}
			data-panel-open={open ? "" : undefined}
			className={cn("min-w-0 text-left", className)}
			onClick={(event) => {
				toggle();
				onClick?.(event);
			}}
			{...props}
		/>
	);
};

/*
A closed panel is unmounted rather than hidden. What these panels hold is a
chart or an editor, and keeping one mounted behind `display:none` means it goes
on measuring and drawing a box nobody is looking at.
*/
export const CollapsiblePanel = ({
	ref,
	className,
	children,
	...props
}: ComponentProps<"div">) => {
	const { open, panelId } = useCollapsible("CollapsiblePanel");

	if (!open) {
		return null;
	}

	return (
		<div
			ref={ref}
			id={panelId}
			data-panel-open=""
			className={cn("min-w-0", className)}
			{...props}
		>
			{children}
		</div>
	);
};

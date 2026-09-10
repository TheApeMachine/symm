"use client";

import { CheckIcon } from "lucide-react";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Checkbox is a real input with a drawn box over it.

The native control stays in the tree, sized to the box and transparent, so
focus, keyboard, form association, and the label's htmlFor all keep working;
the visible parts are siblings that read its state through `peer`. Replacing
the input with a styled div is how a checkbox stops being reachable by anything
but a mouse.

The box and the tick are both siblings of the input rather than the tick being
nested in the box, because `peer-checked:` compiles to a sibling combinator and
reaches no deeper than that.
*/

export type CheckboxProps = Omit<ComponentProps<"input">, "type">;

export const Checkbox = ({ ref, className, ...props }: CheckboxProps) => (
	<span className={cn("relative inline-flex size-3.5 shrink-0", className)}>
		<input
			ref={ref}
			type="checkbox"
			className="peer absolute inset-0 z-10 m-0 cursor-pointer opacity-0 disabled:cursor-not-allowed"
			{...props}
		/>
		<span
			aria-hidden
			className={cn(
				"pointer-events-none absolute inset-0 rounded-[3px]",
				"border border-(--line2) bg-(--sunken) transition-colors",
				"peer-checked:border-(--acc) peer-checked:bg-(--acc)",
				"peer-focus-visible:ring-1 peer-focus-visible:ring-(--acc)",
				"peer-disabled:opacity-50",
			)}
		/>
		<CheckIcon
			aria-hidden
			className={cn(
				"pointer-events-none absolute inset-0 m-auto size-2.5",
				"text-(--bg) opacity-0 peer-checked:opacity-100",
			)}
		/>
	</span>
);

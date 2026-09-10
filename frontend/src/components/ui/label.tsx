import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Label names a control.

Typography.Label is the overline that titles a rail or a stat; this is the
smaller, sentence-cased thing that sits against an input and carries an
htmlFor. They read differently on purpose, so a form does not look like a wall
of section headings.
*/

export const Label = ({
	ref,
	className,
	...props
}: ComponentProps<"label">) => (
	// biome-ignore lint/a11y/noLabelWithoutControl: a primitive cannot contain the control it names; callers pass htmlFor.
	<label
		ref={ref}
		className={cn(
			"select-none text-[10.5px] text-(--f3) leading-tight",
			"peer-disabled:opacity-50",
			className,
		)}
		{...props}
	/>
);

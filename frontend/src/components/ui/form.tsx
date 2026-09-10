import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/*
Form is a group of controls that submits nothing.

The controls inside a node or a panel are edited live — each change is the
change — so there is no submit and no action. It stays a <form> because that is
what groups labelled controls for a screen reader, and it swallows submit so a
stray Enter cannot navigate the page away from an editor.
*/

export const Form = ({
	ref,
	className,
	onSubmit,
	...props
}: ComponentProps<"form">) => (
	<form
		ref={ref}
		className={cn("flex min-w-0 flex-col gap-2", className)}
		onSubmit={(event) => {
			event.preventDefault();
			onSubmit?.(event);
		}}
		{...props}
	/>
);

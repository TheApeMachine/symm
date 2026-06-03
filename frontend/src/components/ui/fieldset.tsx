"use client";

import { Fieldset as FieldsetPrimitive } from "@base-ui/react/fieldset";
import { cn } from "@/lib/utils";

export const Fieldset = ({
	className,
	...props
}: FieldsetPrimitive.Root.Props) => {
	return (
		<FieldsetPrimitive.Root
			className={className}
			data-slot="fieldset"
			{...props}
		/>
	);
};

Fieldset.Legend = ({ className, ...props }: FieldsetPrimitive.Legend.Props) => {
	return (
		<FieldsetPrimitive.Legend
			className={cn("font-semibold text-foreground", className)}
			data-slot="fieldset-legend"
			{...props}
		/>
	);
};

export { FieldsetPrimitive };

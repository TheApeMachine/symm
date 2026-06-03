"use client";

import { Field as FieldPrimitive } from "@base-ui/react/field";
import { cn } from "@/lib/utils";

export const Field = ({ className, ...props }: FieldPrimitive.Root.Props) => {
	return (
		<FieldPrimitive.Root
			className={cn("flex flex-col items-start gap-2", className)}
			data-slot="field"
			{...props}
		/>
	);
};

Field.Label = ({ className, ...props }: FieldPrimitive.Label.Props) => {
	return (
		<FieldPrimitive.Label
			className={cn(
				"inline-flex items-center gap-2 font-medium text-base/4.5 text-foreground data-disabled:opacity-64 sm:text-sm/4",
				className,
			)}
			data-slot="field-label"
			{...props}
		/>
	);
};

Field.Item = ({ className, ...props }: FieldPrimitive.Item.Props) => {
	return (
		<FieldPrimitive.Item
			className={cn("flex", className)}
			data-slot="field-item"
			{...props}
		/>
	);
};

Field.Description = ({
	className,
	...props
}: FieldPrimitive.Description.Props) => {
	return (
		<FieldPrimitive.Description
			className={cn("text-muted-foreground text-xs", className)}
			data-slot="field-description"
			{...props}
		/>
	);
};

Field.Error = ({ className, ...props }: FieldPrimitive.Error.Props) => {
	return (
		<FieldPrimitive.Error
			className={cn("text-destructive-foreground text-xs", className)}
			data-slot="field-error"
			{...props}
		/>
	);
};

export const FieldControl: typeof FieldPrimitive.Control =
	FieldPrimitive.Control;
export const FieldValidity: typeof FieldPrimitive.Validity =
	FieldPrimitive.Validity;

export { FieldPrimitive };

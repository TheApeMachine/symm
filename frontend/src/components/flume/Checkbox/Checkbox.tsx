import React from "react";
import { Checkbox as CheckboxPrimitive } from "#/components/ui/checkbox";
import { Label } from "#/components/ui/label";

interface CheckboxProps {
	label: string;
	data: boolean;
	onChange: (data: boolean) => void;
}

const Checkbox = ({ label, data, onChange }: CheckboxProps) => {
	const id = React.useId();

	return (
		<div className="flex items-start gap-2">
			<CheckboxPrimitive
				checked={data}
				id={id}
				onCheckedChange={(checked) => {
					onChange(checked === true);
				}}
			/>
			<Label className="cursor-pointer font-normal" htmlFor={id}>
				{label}
			</Label>
		</div>
	);
};

export default Checkbox;

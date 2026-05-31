import { memo, useCallback, type ChangeEvent } from "react";

import { Flex } from "#/components/ui/flex";
import {
	fluidVisualParamSpecs,
	type FluidVisualParamKey,
	type FluidVisualParams,
} from "#/lib/symm/fluid-visual-params";

type FluidVisualEditorProps = {
	params: FluidVisualParams;
	onChange: (key: FluidVisualParamKey, value: number) => void;
	onReset: () => void;
};

const formatParamValue = (value: number): string => {
	if (Math.abs(value) >= 10) {
		return value.toFixed(1);
	}

	return value.toFixed(2);
};

export const FluidVisualEditor = memo(function FluidVisualEditor({
	params,
	onChange,
	onReset,
}: FluidVisualEditorProps) {
	const handleChange = useCallback(
		(key: FluidVisualParamKey) => (event: ChangeEvent<HTMLInputElement>) => {
			onChange(key, Number.parseFloat(event.target.value));
		},
		[onChange],
	);

	return (
		<Flex.Column className="fluid-visual-editor">
			<Flex.Row className="items-center justify-between gap-2">
				<small className="text-[10px] font-medium uppercase tracking-wide text-(--dash-muted)">
					Visual params
				</small>
				<button
					type="button"
					className="rounded border border-(--dash-border) px-1.5 py-0.5 text-[9px] text-(--dash-muted) hover:bg-(--dash-row-hover) hover:text-(--dash-text)"
					onClick={onReset}
				>
					Reset
				</button>
			</Flex.Row>
			{fluidVisualParamSpecs.map((spec) => (
				<label
					key={spec.key}
					className="grid grid-cols-[1fr_auto] items-center gap-x-2 gap-y-1"
				>
					<small className="col-span-2 text-[9px] text-(--dash-muted)">
						{spec.label}
					</small>
					<input
						type="range"
						min={spec.min}
						max={spec.max}
						step={spec.step}
						value={params[spec.key]}
						onChange={handleChange(spec.key)}
						className="col-span-1 h-1 w-full cursor-pointer accent-(--dash-accent)"
					/>
					<small className="col-span-1 w-10 text-right font-mono text-[9px] text-(--dash-text)">
						{formatParamValue(params[spec.key])}
					</small>
				</label>
			))}
		</Flex.Column>
	);
});

import React from "react";
import Checkbox from "#/components/flume/Checkbox/Checkbox";
import {
	ContextContext,
	NodeActionsContext,
	RecalculateStageRectContext,
} from "#/components/flume/context";
import Select from "#/components/flume/Select/Select";
import type {
	ControlData,
	ControlRenderCallback,
	ControlTypes,
	InputData,
	SelectOption,
	ValueSetter,
} from "#/components/flume/types";
import { Input as InputPrimitive } from "#/components/ui/input";
import styles from "./Control.module.css";

interface CommonProps {
	name: string;
	nodeId: string;
	portName: string;
	label: string;
	inputLabel: string;
	allData: ControlData;
	inputData: InputData;
	triggerRecalculation: () => void;
	updateNodeConnections: () => void;
	setValue?: ValueSetter;
	isMonoControl?: boolean;
}

interface TextInputProps extends CommonProps {
	type: "text";
	data: string;
	defaultValue?: string;
	placeholder?: string;
}

interface NumberInputProps extends CommonProps {
	type: "number";
	data: number;
	defaultValue?: number;
	step?: number;
	placeholder?: string;
}

interface CheckboxProps extends CommonProps {
	type: "checkbox";
	data: boolean;
	defaultValue?: boolean;
}

interface SelectProps extends CommonProps {
	type: "select";
	data: string;
	options: SelectOption[];
	defaultValue?: string;
	placeholder?: string;
	getOptions?: (
		inputData: InputData,
		executionContext: unknown,
	) => SelectOption[];
}

interface MultiSelectProps extends CommonProps {
	type: "multiselect";
	data: string[];
	options: SelectOption[];
	defaultValue?: string[];
	placeholder?: string;
	getOptions?: (
		inputData: InputData,
		executionContext: unknown,
	) => SelectOption[];
}

interface CustomProps extends CommonProps {
	type: "custom";
	data: unknown;
	defaultValue?: unknown;
	render?: ControlRenderCallback;
}

type ControlProps =
	| TextInputProps
	| NumberInputProps
	| CheckboxProps
	| SelectProps
	| MultiSelectProps
	| CustomProps;

const Control = (props: ControlProps) => {
	const {
		type,
		name,
		nodeId,
		portName,
		label,
		inputLabel,
		data,
		allData,
		inputData,
		triggerRecalculation,
		updateNodeConnections,
		setValue,
		defaultValue,
		isMonoControl,
	} = props;
	const nodeActions = React.useContext(NodeActionsContext);
	const executionContext = React.useContext(ContextContext);
	const recalculateStageRect = React.useContext(RecalculateStageRectContext);

	const handleResizeMouseDown = React.useCallback(
		(event: React.MouseEvent<HTMLElement>) => {
			event.stopPropagation();
			recalculateStageRect?.();
			const handleMouseMove = (e: MouseEvent) => {
				e.stopPropagation();
				updateNodeConnections();
			};
			const handleDragEnd = () => {
				document.removeEventListener("mousemove", handleMouseMove);
				document.removeEventListener("mouseup", handleDragEnd);
			};
			document.addEventListener("mousemove", handleMouseMove);
			document.addEventListener("mouseup", handleDragEnd);
		},
		[recalculateStageRect, updateNodeConnections],
	);

	const calculatedLabel = isMonoControl ? inputLabel : label;

	const onChange = (data: unknown) => {
		nodeActions?.setPortData({
			data,
			nodeId,
			portName,
			controlName: name,
			setValue,
		});
		triggerRecalculation();
	};

	const getControlByType = (type: ControlTypes) => {
		const commonProps = {
			triggerRecalculation,
			updateNodeConnections,
			onChange,
		} as const;
		switch (type) {
			case "select": {
				const { options, getOptions, placeholder } = props as SelectProps;

				return (
					<Select
						onChange={commonProps.onChange as (d: string | string[]) => void}
						data={props.data as string}
						options={
							getOptions ? getOptions(inputData, executionContext) : options
						}
						placeholder={placeholder}
					/>
				);
			}
			case "text": {
				const { placeholder } = props as TextInputProps;

				return (
					<InputPrimitive
						className="w-full min-w-0"
						data-flume-component="text-input-text"
						value={(props.data as string) ?? ""}
						onDragStart={(e) => {
							e.stopPropagation();
						}}
						onMouseDown={handleResizeMouseDown}
						onValueChange={(next) => {
							commonProps.onChange(next);
						}}
						placeholder={placeholder}
						size="sm"
					/>
				);
			}
			case "number": {
				const { step, placeholder } = props as NumberInputProps;

				return (
					<InputPrimitive
						className="w-full min-w-0"
						data-flume-component="text-input-number"
						value={(props.data as number) ?? 0}
						onDragStart={(e) => {
							e.stopPropagation();
						}}
						onMouseDown={handleResizeMouseDown}
						onKeyDown={(e) => {
							if (e.key === "e" || e.key === "E") {
								e.preventDefault();
							}
						}}
						onBlur={(e) => {
							if (!e.target.value) {
								commonProps.onChange(0);
							}
						}}
						onValueChange={(raw) => {
							const inputValue = raw.replace(/e/g, "");
							if (!inputValue) {
								return;
							}
							const value = parseFloat(inputValue);
							commonProps.onChange(Number.isNaN(value) ? 0 : value);
						}}
						step={step ?? 1}
						type="number"
						placeholder={placeholder}
						size="sm"
					/>
				);
			}
			case "checkbox":
				return (
					<Checkbox
						onChange={commonProps.onChange as (d: boolean) => void}
						data={props.data as boolean}
						label={calculatedLabel}
					/>
				);
			case "multiselect": {
				const { options, getOptions, placeholder } = props as MultiSelectProps;

				return (
					<Select
						allowMultiple
						onChange={commonProps.onChange as (d: string | string[]) => void}
						data={props.data as string[]}
						options={
							getOptions ? getOptions(inputData, executionContext) : options
						}
						placeholder={placeholder}
					/>
				);
			}
			case "custom": {
				const { render } = props as CustomProps;

				return (
					render?.(
						data,
						onChange,
						executionContext,
						triggerRecalculation,
						{
							label,
							name,
							portName,
							inputLabel,
							defaultValue,
						},
						allData,
					) ?? null
				);
			}
			default:
				return <div>Control</div>;
		}
	};

	return (
		<div className={styles.wrapper} data-flume-component="control">
			{calculatedLabel && type !== "checkbox" && type !== "custom" && (
				<span
					data-flume-component="control-label"
					className={styles.controlLabel}
				>
					{calculatedLabel}
				</span>
			)}
			{getControlByType(type)}
		</div>
	);
};

export default Control;

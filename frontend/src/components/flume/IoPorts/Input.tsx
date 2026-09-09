import React from "react";
import Control from "#/components/flume/Control/Control";
import type {
	ControlData,
	Control as ControlType,
	InputData,
	PortTypeMap,
} from "#/components/flume/types";
import usePrevious from "#/hooks/usePrevious";
import { cn } from "@/lib/utils";
import styles from "./IoPorts.module.css";
import Port from "./Port";

/*
Input renders a single input row: optional port handle, optional label,
and the control(s) shown when the port is unconnected. The renderInputControl
switch is the only place that needs TS to narrow Control's discriminated
union — every other concern is pulled out into Port / usePortDrag.
*/

interface InputProps {
	type: string;
	label: string;
	name: string;
	nodeId: string;
	data: ControlData;
	controls: ControlType[];
	inputTypes: PortTypeMap;
	noControls?: boolean;
	triggerRecalculation: () => void;
	updateNodeConnections: () => void;
	isConnected?: boolean;
	inputData: InputData;
	hidePort?: boolean;
}

interface RenderControlOptions {
	control: ControlType;
	data: ControlData;
	nodeId: string;
	portName: string;
	label: string;
	inputData: InputData;
	isMonoControl: boolean;
	triggerRecalculation: () => void;
	updateNodeConnections: () => void;
}

const renderInputControl = ({
	control,
	data,
	nodeId,
	portName,
	label,
	inputData,
	isMonoControl,
	triggerRecalculation,
	updateNodeConnections,
}: RenderControlOptions): React.ReactNode => {
	const shared = {
		nodeId,
		portName,
		triggerRecalculation,
		updateNodeConnections,
		inputLabel: label,
		allData: data,
		inputData,
		isMonoControl,
	} as const;

	switch (control.type) {
		case "text": {
			const value =
				(data[control.name] as string | undefined) ?? control.defaultValue;
			return (
				<Control key={control.name} {...control} {...shared} data={value} />
			);
		}
		case "number": {
			const value =
				(data[control.name] as number | undefined) ?? control.defaultValue;
			return (
				<Control key={control.name} {...control} {...shared} data={value} />
			);
		}
		case "checkbox": {
			const value =
				(data[control.name] as boolean | undefined) ?? control.defaultValue;
			return (
				<Control key={control.name} {...control} {...shared} data={value} />
			);
		}
		case "select": {
			const value =
				(data[control.name] as string | undefined) ?? control.defaultValue;
			return (
				<Control key={control.name} {...control} {...shared} data={value} />
			);
		}
		case "multiselect": {
			const value =
				(data[control.name] as string[] | undefined) ?? control.defaultValue;
			return (
				<Control key={control.name} {...control} {...shared} data={value} />
			);
		}
		case "custom": {
			const value = data[control.name];
			return (
				<Control key={control.name} {...control} {...shared} data={value} />
			);
		}
		default:
			return null;
	}
};

const Input = ({
	type,
	label,
	name,
	nodeId,
	data,
	controls: localControls,
	inputTypes,
	noControls,
	triggerRecalculation,
	updateNodeConnections,
	isConnected,
	inputData,
	hidePort,
}: InputProps) => {
	const {
		label: defaultLabel,
		color,
		controls: defaultControls = [],
	} = inputTypes[type] || {};
	const prevConnected = usePrevious(isConnected);
	const controls = localControls || defaultControls;

	React.useEffect(() => {
		if (isConnected !== prevConnected) {
			triggerRecalculation();
		}
	}, [isConnected, prevConnected, triggerRecalculation]);

	const showLabel = !controls.length || noControls || isConnected;
	const showControls = !noControls && !isConnected;
	const isMonoControl = controls.length === 1;

	return (
		<fieldset
			data-flume-component="port-input"
			className={cn(styles.transput, "border-0 p-0")}
			data-controlless={isConnected || noControls || !controls.length}
			onDragStart={(e) => {
				e.preventDefault();
				e.stopPropagation();
			}}
		>
			{!hidePort ? (
				<Port
					type={type}
					color={color}
					name={name}
					nodeId={nodeId}
					isInput
					triggerRecalculation={triggerRecalculation}
				/>
			) : null}
			{showLabel ? (
				<span data-flume-component="port-label" className={styles.portLabel}>
					{label || defaultLabel}
				</span>
			) : null}
			{showControls ? (
				<div className={styles.controls}>
					{controls.map((control) =>
						renderInputControl({
							control,
							data,
							nodeId,
							portName: name,
							label,
							inputData,
							isMonoControl,
							triggerRecalculation,
							updateNodeConnections,
						}),
					)}
				</div>
			) : null}
		</fieldset>
	);
};

export default Input;

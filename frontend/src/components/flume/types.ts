import type * as React from "react";
import type { ReactNode } from "react";

export type ControlData = { [controlName: string]: unknown };
export type InputData = { [portName: string]: ControlData };

export type ControlTypes =
	| "text"
	| "number"
	| "select"
	| "checkbox"
	| "multiselect"
	| "custom";

export type ValueSetter = (newData: unknown, oldData: unknown) => unknown;

export interface GenericControl {
	type: ControlTypes;
	label: string;
	name: string;
	defaultValue: unknown;
	setValue?: ValueSetter;
}

export interface TextControl extends GenericControl {
	type: "text";
	defaultValue: string;
}

export interface SelectOption {
	label: string;
	value: string;
	description?: string;
	sortIndex?: number;
	/** When set on any option in a menu, options are grouped under this label in the picker UI. */
	category?: string;
	node?: NodeType;
	internalType?: "comment";
}

export interface SelectControl extends GenericControl {
	type: "select";
	options: SelectOption[];
	defaultValue: string;
	getOptions?: (inputData: InputData, context: unknown) => SelectOption[];
	placeholder?: string;
}

export interface NumberControl extends GenericControl {
	type: "number";
	defaultValue: number;
	step?: number;
}

export interface CheckboxControl extends GenericControl {
	type: "checkbox";
	defaultValue: boolean;
}

export interface MultiselectControl extends GenericControl {
	type: "multiselect";
	options: SelectOption[];
	defaultValue: string[];
	getOptions?: (inputData: InputData, context: unknown) => SelectOption[];
	placeholder?: string;
}

export type ControlRenderCallback = (
	data: unknown,
	onChange: (newData: unknown) => void,
	context: unknown,
	redraw: () => void,
	portProps: {
		label: string;
		name: string;
		portName: string;
		inputLabel: string;
		defaultValue: unknown;
	},
	controlData: ControlData,
) => ReactNode;

export interface CustomControl extends GenericControl {
	type: "custom";
	defaultValue: unknown;
	render: ControlRenderCallback;
}

export type Control =
	| TextControl
	| SelectControl
	| NumberControl
	| CheckboxControl
	| MultiselectControl
	| CustomControl;

export type Colors =
	| "yellow"
	| "orange"
	| "red"
	| "pink"
	| "purple"
	| "blue"
	| "green"
	| "grey";

export interface PortType {
	/**
	 * A unique string identifier for the port. Preferred to be camelCased.
	 */
	type: string;
	/**
	 * A default string identifier used when the port is constructed.
	 */
	name: string;
	/**
	 * A default human-readable label for the port.
	 */
	label: string;
	/**
	 * When true the port will not render its controls.
	 *
	 * @defaultValue false
	 */
	noControls: boolean;
	/**
	 * The color of the port. Should be one of the colors defined by the Colors type.
	 */
	color: Colors;
	/**
	 * If true the ports controls will render but the actual port will not. This disallows connections.
	 *
	 * @defaultValue false
	 */
	hidePort: boolean;
	/**
	 * An array of controls to render on the port.
	 */
	controls: Control[];
	/**
	 * An array of port type strings. Only port types included in this array will be allowed to connect to this port. By default ports always accept their own type.
	 */
	acceptTypes?: string[];
}

export type PortTypeMap = { [portType: string]: PortType };

export type PortTypeBuilder = (config?: Partial<PortType>) => PortType;

export interface PortTypeConfig extends Partial<PortType> {
	type: string;
	name: string;
}

export type TransputType = "input" | "output";

export type TransputBuilder = (
	inputData: InputData,
	connections: Connections,
	context: unknown,
) => PortType[];

export interface NodeType {
	/**
	 * A unique randomly-generated string identifier for the node.
	 */
	id: string;
	/**
	 * A unique string identifier for the node. Preferred to be camelCased.
	 */
	type: string;
	/**
	 * A human-readable label for the node.
	 */
	label: string;
	/**
	 * A human-readable description for the node. Renders in the "Add Node" context menu.
	 * Optional.
	 */
	description?: string;
	/**
	 * If false the node may not be added to the canvas.
	 * Optional; when omitted, nodes are addable.
	 *
	 * @defaultValue true
	 */
	addable?: boolean;
	/**
	 * If false the node may not be removed from the canvas.
	 * Optional; when omitted, nodes are deletable.
	 *
	 * @defaultValue true
	 */
	deletable?: boolean;
	inputs: PortType[] | TransputBuilder;
	outputs: PortType[] | TransputBuilder;
	initialWidth?: number;
	sortIndex?: number;
	/** Optional section title for context menus and other grouped pickers (e.g. "Hugging Face"). */
	category?: string;
	root?: boolean;
	/** Pre-wired sub-graph seeded when this node type is first added to the canvas. */
	defaultSubGraph?: NodeMap;
}

export type NodeTypeMap = { [nodeType: string]: NodeType };

export type DynamicPortTypeBuilder = (
	inputData: InputData,
	connections: Connections,
	context: unknown,
) => PortType[];

export interface NodeTypeConfig
	extends Omit<Partial<NodeType>, "inputs" | "outputs"> {
	type: string;
	/**
	 * Represents the ports available to be connected as inputs to the node. Must be one of the following types:
	 * - An array of ports
	 * - A function that returns an array of ports at definition time
	 * - A function that returns a function that returns an array of ports at runtime
	 */
	inputs?:
		| PortType[]
		| ((ports: { [portType: string]: PortTypeBuilder }) => PortType[])
		| ((ports: {
				[portType: string]: PortTypeBuilder;
		  }) => DynamicPortTypeBuilder);
	/**
	 * Represents the ports available to be connected as outputs from the node. Must be one of the following types:
	 * - An array of ports
	 * - A function that returns an array of ports at definition time
	 * - A function that returns a function that returns an array of ports at runtime
	 */
	outputs?:
		| PortType[]
		| ((ports: { [portType: string]: PortTypeBuilder }) => PortType[])
		| ((ports: {
				[portType: string]: PortTypeBuilder;
		  }) => DynamicPortTypeBuilder);
}

export type Connection = {
	nodeId: string;
	portName: string;
};

export type ConnectionMap = { [portName: string]: Connection[] };

export type Connections = {
	inputs: ConnectionMap;
	outputs: ConnectionMap;
};

export type FlumeNode = {
	id: string;
	type: string;
	width: number;
	height?: number;
	x: number;
	y: number;
	inputData: InputData;
	connections: Connections;
	defaultNode?: boolean;
	root?: boolean;
	subGraph?: NodeMap;
};

export type DefaultNode = {
	type: string;
	x?: number;
	y?: number;
};

export type DefaultConnection = {
	output: { nodeType: string; portName: string };
	input: { nodeType: string; portName: string };
};

export type NodeMap = { [nodeId: string]: FlumeNode };

export type ToastTypes = "danger" | "info" | "success" | "warning";

export type FlumeComment = {
	id: string;
	text: string;
	x: number;
	y: number;
	width: number;
	height: number;
	color: Colors;
	isNew: boolean;
};

export type FlumeCommentMap = { [commentId: string]: FlumeComment };

export type StageTranslate = {
	x: number;
	y: number;
};

export type Coordinate = {
	x: number;
	y: number;
};

export type StageState = {
	scale: number;
	translate: StageTranslate;
};

export type CircularBehavior = "prevent" | "warn" | "allow";

export type NodeHeaderActions = {
	openMenu: (event: MouseEvent | React.MouseEvent) => void;
	closeMenu: () => void;
	deleteNode: () => void;
};

export type NodeHeaderRenderCallback = (
	Wrapper: React.ComponentType<React.PropsWithChildren>,
	nodeType: NodeType,
	actions: NodeHeaderActions,
) => ReactNode;

export type PortResolver = (
	portType: string,
	data: ControlData,
	context: unknown,
) => unknown;

export type NodeResolver = (
	node: FlumeNode,
	inputValues: Record<string, unknown>,
	nodeType: NodeType,
	context: unknown,
) => Record<string, unknown> | Promise<Record<string, unknown>>;

export interface RootEngineOptions {
	rootNodeId?: string;
	context?: unknown;
	maxLoops?: number;
	onlyResolveConnected?: boolean;
}

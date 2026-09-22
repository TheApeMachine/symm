import React from "react";
import { RingBuffer } from "#/collections/ring";
import componentMetadata from "./ui-component-metadata.generated.json";
import {
	type UIComponentName,
	uiComponents,
} from "./ui-component-registry.generated";

/*
The graph carries one scalar per evaluation, so a component that draws a
series keeps its own history. The ring is held against the producer rather
than the component reading it, so two views of the same metric share one
history instead of each starting empty.

A sparkline draws into a 150-unit viewBox, so it holds a point per unit; past
that the oldest reading leaves as the newest arrives.
*/
const SERIES_CAPACITY = 150;

const seriesRings = new Map<string, RingBuffer<number>>();

const seriesKey = (node: string, port: string) => `${node}.${port}`;

const readSeries = (key: string): number[] =>
	seriesRings.get(key)?.toArray() ?? [];

const recordSeries = (key: string, value: number) => {
	let ring = seriesRings.get(key);

	if (!ring) {
		ring = new RingBuffer<number>(SERIES_CAPACITY);
		seriesRings.set(key, ring);
	}

	ring.add(value);
};

/** Forgets every accumulated history. Tests start from nothing. */
export const clearSeries = () => seriesRings.clear();

/** The history held against one producer's port. */
export const readSeriesForTest = (key: string): number[] => readSeries(key);

const seriesProps = (componentName: string): Set<string> => {
	const meta = (componentMetadata as Record<string, { props?: Array<{ name: string; type: string }> }>)[
		componentName
	];

	return new Set(
		(meta?.props ?? [])
			.filter((prop) => prop.type === "series")
			.map((prop) => prop.name),
	);
};

type SeriesBinding = { prop: string; key: string; value: number | undefined };

/*
SeriesBound draws a component whose data arrives a scalar at a time.

Recording happens after the render that observed the value, never during it,
so a repeated render cannot enter the same reading twice.
*/
const SeriesBound = ({
	component: Component,
	props,
	bindings,
	children,
}: {
	component: React.ComponentType<any>;
	props: Record<string, any>;
	bindings: SeriesBinding[];
	children?: React.ReactNode;
}) => {
	const [, observed] = React.useReducer((count: number) => count + 1, 0);
	const readings = bindings.map((binding) => binding.value).join(",");

	React.useEffect(() => {
		let recorded = false;

		for (const binding of bindings) {
			if (typeof binding.value !== "number") continue;

			recordSeries(binding.key, binding.value);
			recorded = true;
		}

		if (recorded) observed();
		// The readings are what changed; the bindings array is rebuilt each render.
	}, [readings]);

	const withHistory = { ...props };

	for (const binding of bindings) {
		withHistory[binding.prop] = readSeries(binding.key);
	}

	return children ? (
		<Component {...withHistory}>{children}</Component>
	) : (
		<Component {...withHistory} />
	);
};

/*
CompiledUINode represents one node in a compiled UI subgraph.
Layout is expressed strictly through standard structural components (Flex, Panel, Grid)
and normal className styling; no bespoke positioning matrices exist.
*/
export interface CompiledUINode {
	name: UIComponentName | string;
	className?: string;
	props?: Record<string, any>;
	children?: CompiledUINode[];
}

/*
CompiledUIRoute describes a route and its root UI component subgraph.
*/
export interface CompiledUIRoute {
	path: string;
	title?: string;
	components: CompiledUINode[];
}

/*
resolveBindings extracts live values from observableState for binding objects:
{ binding: { node: "some_metric", port: "out" } }
If the prop value is a CompiledUINode (such as a structural slot), it renders it.
*/
export function resolveBindings(
	props: Record<string, any>,
	observableState?: Record<string, any>,
): Record<string, any> {
	const resolved: Record<string, any> = {};

	for (const [key, val] of Object.entries(props)) {
		if (
			val !== null &&
			typeof val === "object" &&
			"binding" in val &&
			typeof val.binding === "object" &&
			val.binding !== null
		) {
			const b = val.binding as { node: string; port?: string; field?: string };
			const fieldKey = b.port ?? b.field ?? "out";
			const liveVal = observableState?.[b.node]?.[fieldKey];
			resolved[key] = liveVal !== undefined ? liveVal : undefined;
			continue;
		}

		if (
			val !== null &&
			typeof val === "object" &&
			"name" in val &&
			typeof val.name === "string" &&
			val.name in uiComponents
		) {
			resolved[key] = renderNode(val as CompiledUINode, key, observableState);
			continue;
		}

		resolved[key] = val;
	}

	return resolved;
}

/*
renderNode recursively renders a compiled UI subgraph through the generated component registry.
Per UI.md:
1. Resolve the component name through the generated registry.
2. Resolve configured prop values and live data bindings.
3. Find structural child nodes.
4. Render children recursively.
5. Pass configured props and children to the component.
6. Fail clearly on unknown components rather than silently rendering nothing.
*/
export function renderNode(
	node: CompiledUINode,
	key?: string | number,
	observableState?: Record<string, any>,
): React.ReactNode {
	const Component = uiComponents[node.name as UIComponentName];

	if (!Component) {
		throw new Error(
			`UI Graph: unknown component "${node.name}". Component must be registered in uiComponents.`,
		);
	}

	let resolvedProps: Record<string, any> = {};

	if (node.props) {
		resolvedProps = resolveBindings(node.props, observableState);
	}

	if (node.className) {
		resolvedProps.className = resolvedProps.className
			? `${resolvedProps.className} ${node.className}`
			: node.className;
	}

	// A prop the component draws as a series takes its readings one at a
	// time, so it is handed the history rather than the latest scalar.
	const series = seriesProps(node.name);
	const bindings: SeriesBinding[] = [];

	if (series.size > 0 && node.props) {
		for (const [prop, authored] of Object.entries(node.props)) {
			if (!series.has(prop)) continue;

			const binding = (authored as { binding?: { node: string; port?: string } })
				?.binding;

			if (!binding) continue;

			bindings.push({
				prop,
				key: seriesKey(binding.node, binding.port ?? "out"),
				value: resolvedProps[prop],
			});

			delete resolvedProps[prop];
		}
	}

	const childNodes = node.children ?? [];
	const renderedChildren =
		childNodes.length > 0
			? childNodes.map((child, index) =>
					renderNode(
						child,
						key !== undefined ? `${key}-${index}` : index,
						observableState,
					),
				)
			: undefined;

	if (bindings.length > 0) {
		return (
			<SeriesBound
				key={key}
				component={Component}
				props={resolvedProps}
				bindings={bindings}
			>
				{renderedChildren}
			</SeriesBound>
		);
	}

	if (renderedChildren) {
		return (
			<Component key={key} {...resolvedProps}>
				{renderedChildren}
			</Component>
		);
	}

	return <Component key={key} {...resolvedProps} />;
}

/*
renderUIRoute renders a route's complete component tree.
*/
export function renderUIRoute(
	route: CompiledUIRoute,
	observableState?: Record<string, any>,
): React.ReactNode {
	const nodes = route.components ?? [];

	return (
		<div className="flex h-full w-full flex-col" data-route-path={route.path}>
			{nodes.map((node, index) => renderNode(node, index, observableState))}
		</div>
	);
}

import type React from "react";
import { useCallback, useMemo, useState } from "react";
import {
	type UIComponentName,
	uiComponents,
} from "./ui-component-registry.generated";

/*
StateBinding names a piece of route state and the value a node cares about.
*/
export interface StateBinding {
	key: string;
	value: unknown;
}

/*
RouteState is the choice a surface is currently showing, and the way to change
it. A graph-drawn surface holds no state of its own; the route holds it, and
nodes read and write it by name.
*/
export interface RouteState {
	values: Record<string, unknown>;
	select: (key: string, value: unknown) => void;
	/* What the running program last delivered to each node's ports, by node id. */
	bound?: Record<string, Record<string, unknown>>;
}

/*
CompiledUINode represents one node in a compiled UI subgraph.
Layout is expressed strictly through standard structural components (Flex, Panel, Grid)
and normal className styling; no bespoke positioning matrices exist.
*/
export interface CompiledUINode {
	/* The node id the component was authored with. */
	id?: string;
	name: UIComponentName | string;
	className?: string;
	props?: Record<string, any>;
	children?: CompiledUINode[];
	/* Clicking this node makes this choice. */
	selects?: StateBinding;
	/* This node is drawn only while this choice stands. */
	visibleWhen?: StateBinding;
}

/*
CompiledUIRoute describes a route and its root UI component subgraph.
*/
export interface CompiledUIRoute {
	path: string;
	title?: string;
	components: CompiledUINode[];
	/* The choices this surface starts on, declared by its state nodes. */
	state?: Record<string, unknown>;
}

/*
resolveBindings extracts live values from observableState for binding objects:
{ binding: { node: "some_metric", port: "out" } }
If the prop value is a CompiledUINode (such as a structural slot), it renders it.
*/
export const resolveBindings = (
	props: Record<string, any>,
	observableState?: Record<string, any>,
	sources?: Record<string, unknown>,
	state?: RouteState,
): Record<string, any> => {
	const resolved: Record<string, any> = {};

	for (const [key, val] of Object.entries(props)) {
		if (
			val !== null &&
			typeof val === "object" &&
			"binding" in val &&
			typeof val.binding === "object" &&
			val.binding !== null
		) {
			const b = val.binding as {
				node?: string;
				port?: string;
				field?: string;
				source?: string;
			};

			// A source is live data the surface was handed, named by the
			// graph rather than produced inside it.
			if (b.source) {
				resolved[key] = sources?.[b.source];
				continue;
			}

			const fieldKey = b.port ?? b.field ?? "out";
			const liveVal = observableState?.[b.node ?? ""]?.[fieldKey];
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
			resolved[key] = renderNode(
				val as CompiledUINode,
				key,
				observableState,
				sources,
				state,
			);
			continue;
		}

		resolved[key] = val;
	}

	return resolved;
};

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
export const renderNode = (
	node: CompiledUINode,
	key?: string | number,
	observableState?: Record<string, any>,
	sources?: Record<string, unknown>,
	state?: RouteState,
): React.ReactNode => {
	// A node that appears under a choice is not drawn while another choice
	// stands. It is left out entirely rather than hidden, so nothing it holds
	// is mounted or subscribed behind a panel nobody is looking at.
	if (node.visibleWhen && state) {
		if (state.values[node.visibleWhen.key] !== node.visibleWhen.value) {
			return null;
		}
	}

	const Component = uiComponents[node.name as UIComponentName];

	if (!Component) {
		throw new Error(
			`UI Graph: unknown component "${node.name}". Component must be registered in uiComponents.`,
		);
	}

	let resolvedProps: Record<string, any> = {};

	if (node.props) {
		resolvedProps = resolveBindings(
			node.props,
			observableState,
			sources,
			state,
		);
	}

	// A port wired to a producer in the running program shows what arrived.
	const delivered = node.id ? state?.bound?.[node.id] : undefined;

	if (delivered) {
		resolvedProps = { ...resolvedProps, ...delivered };
	}

	// A node that makes a choice answers a click with it, and reports whether
	// its own choice is the one standing. Components that carry an `active`
	// prop show that without being told twice.
	if (node.selects && state) {
		const { key: stateKey, value } = node.selects;
		resolvedProps.onClick = () => state.select(stateKey, value);
		resolvedProps.active = state.values[stateKey] === value;
	}

	if (node.className) {
		resolvedProps.className = resolvedProps.className
			? `${resolvedProps.className} ${node.className}`
			: node.className;
	}

	const childNodes = node.children ?? [];
	const renderedChildren =
		childNodes.length > 0
			? childNodes.map((child, index) =>
					renderNode(
						child,
						key !== undefined ? `${key}-${index}` : index,
						observableState,
						sources,
						state,
					),
				)
			: undefined;

	if (renderedChildren) {
		return (
			<Component key={key} {...resolvedProps}>
				{renderedChildren}
			</Component>
		);
	}

	return <Component key={key} {...resolvedProps} />;
};

/*
UIRouteView draws a route and holds the choices it is currently showing.

The state lives here rather than in any component because no component drawn
from a graph owns it: a tab strip and the pane it reveals are separate nodes
that only have the choice in common, and the route is the one thing they are
both inside.
*/
export const UIRouteView = ({
	route,
	observableState,
	sources,
	bound,
}: {
	route: CompiledUIRoute;
	observableState?: Record<string, any>;
	sources?: Record<string, unknown>;
	bound?: Record<string, Record<string, unknown>>;
}) => {
	const [values, setValues] = useState<Record<string, unknown>>(
		() => route.state ?? {},
	);

	const select = useCallback((key: string, value: unknown) => {
		setValues((current) =>
			current[key] === value ? current : { ...current, [key]: value },
		);
	}, []);

	const state = useMemo<RouteState>(
		() => ({ values, select, bound }),
		[values, select, bound],
	);

	const nodes = route.components ?? [];

	return (
		<div className="flex h-full w-full flex-col" data-route-path={route.path}>
			{nodes.map((node, index) =>
				renderNode(node, index, observableState, sources, state),
			)}
		</div>
	);
};

/*
renderUIRoute renders a route's complete component tree.
*/
export const renderUIRoute = (
	route: CompiledUIRoute,
	observableState?: Record<string, any>,
	sources?: Record<string, unknown>,
	bound?: Record<string, Record<string, unknown>>,
): React.ReactNode => (
	<UIRouteView
		route={route}
		observableState={observableState}
		sources={sources}
		bound={bound}
	/>
);

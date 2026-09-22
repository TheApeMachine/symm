import type React from "react";
import {
	type UIComponentName,
	uiComponents,
} from "./ui-component-registry.generated";

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

	const childNodes = node.children ?? [];
	if (childNodes.length > 0) {
		const renderedChildren = childNodes.map((child, index) =>
			renderNode(
				child,
				key !== undefined ? `${key}-${index}` : index,
				observableState,
			),
		);

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

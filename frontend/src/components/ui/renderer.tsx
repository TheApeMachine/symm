import type React from "react";
import { type UIComponentName, uiComponents } from "./ui-component-registry.generated";

/*
CompiledUINode represents one node in a compiled UI subgraph.
Layout is expressed strictly through standard structural components (Flex, Panel, Grid)
and normal className styling; no bespoke positioning matrices exist.
*/
export interface CompiledUINode {
	name: string;
	className?: string;
	props?: Record<string, any>;
	propsJson?: string;
	children?: CompiledUINode[];
	components?: CompiledUINode[];
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
renderNode recursively renders a compiled UI subgraph through the generated component registry.
Per UI.md:
1. Resolve the component name through the generated registry.
2. Resolve configured prop values.
3. Find structural child nodes.
4. Render children recursively.
5. Pass configured props and children to the component.
6. Fail clearly on unknown components rather than silently rendering nothing.
*/
export function renderNode(
	node: CompiledUINode,
	key?: string | number,
): React.ReactNode {
	const Component = uiComponents[node.name as UIComponentName];

	if (!Component) {
		throw new Error(
			`UI Graph: unknown component "${node.name}". Component must be registered in uiComponents.`,
		);
	}

	let resolvedProps: Record<string, any> = {};

	if (node.props) {
		resolvedProps = { ...node.props };
	} else if (node.propsJson) {
		try {
			resolvedProps = JSON.parse(node.propsJson);
		} catch (err) {
			throw new Error(
				`UI Graph: failed to parse propsJson for component "${node.name}": ${(err as Error).message}`,
			);
		}
	}

	if (node.className) {
		resolvedProps.className = resolvedProps.className
			? `${resolvedProps.className} ${node.className}`
			: node.className;
	}

	const childNodes = node.children ?? node.components ?? [];
	if (childNodes.length > 0) {
		const renderedChildren = childNodes.map((child, index) =>
			renderNode(child, key !== undefined ? `${key}-${index}` : index),
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
export function renderUIRoute(route: CompiledUIRoute): React.ReactNode {
	const nodes = route.components ?? [];

	return (
		<div className="flex h-full w-full flex-col" data-route-path={route.path}>
			{nodes.map((node, index) => renderNode(node, index))}
		</div>
	);
}

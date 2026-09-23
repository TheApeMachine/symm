import { useEffect, useMemo, useState } from "react";
import { useGraphResults } from "#/components/flume/graph-results.store";
import type { FlumeNode } from "#/components/flume/types";
import { Alert } from "#/components/ui/alert";
import { compileUI } from "#/components/ui/compiler";
import { Flex } from "#/components/ui/flex";
import { renderUIRoute } from "#/components/ui/renderer";
import { Spinner } from "#/components/ui/spinner";
import { Typography } from "#/components/ui/typography";
import { fetchDefinition, fetchDefinitions } from "#/service/compute";
import { useLiveSources } from "./sources";

export type GraphState =
	| { status: "loading" }
	| { status: "failed"; reason: string }
	| { status: "loaded"; nodes: Record<string, FlumeNode> };

/*
useHubGraph reads one graph the hub holds and re-reads it when the name changes.

A surface being rebuilt is edited on the other side of this fetch, so the graph
is read on every visit rather than cached: the point of these routes is to watch
a graph change, and a cache would keep showing the shape it had when it was
first opened.
*/
export const useHubGraph = (name: string | undefined): GraphState => {
	const [state, setState] = useState<GraphState>({ status: "loading" });

	useEffect(() => {
		if (!name) {
			return;
		}

		let live = true;
		setState({ status: "loading" });

		fetchDefinition(name)
			.then((graph) => {
				if (live) {
					setState({
						status: "loaded",
						nodes: (graph.nodes ?? {}) as Record<string, FlumeNode>,
					});
				}
			})
			.catch((error: Error) => {
				if (live) {
					setState({ status: "failed", reason: error.message });
				}
			});

		return () => {
			live = false;
		};
	}, [name]);

	return state;
};

/*
useHubGraphs lists the graphs the hub holds, and the path each one declares.

The listing is what makes a graph-drawn surface discoverable at all: a surface
exists because a node says so, and until something reads those nodes there is no
index of them anywhere.
*/
export const useHubGraphs = (): {
	status: "loading" | "ready" | "failed";
	graphs: Array<{ name: string; path?: string; title?: string }>;
	reason?: string;
} => {
	const [state, setState] = useState<{
		status: "loading" | "ready" | "failed";
		graphs: Array<{ name: string; path?: string; title?: string }>;
		reason?: string;
	}>({ status: "loading", graphs: [] });

	useEffect(() => {
		let live = true;

		fetchDefinitions()
			.then(async (names) => {
				const graphs = await Promise.all(
					names.map(async (name) => {
						const graph = await fetchDefinition(name).catch(() => null);
						const nodes = (graph?.nodes ?? {}) as Record<string, FlumeNode>;

						for (const node of Object.values(nodes)) {
							if ((node as { type?: string }).type !== "ui.UIRoute") {
								continue;
							}

							const data = (node as { inputData?: Record<string, any> })
								.inputData;

							return {
								name,
								path: data?.path?.value ?? data?.path,
								title: data?.title?.value ?? data?.title,
							};
						}

						return { name };
					}),
				);

				if (live) {
					setState({ status: "ready", graphs });
				}
			})
			.catch((error: Error) => {
				if (live) {
					setState({ status: "failed", graphs: [], reason: error.message });
				}
			});

		return () => {
			live = false;
		};
	}, []);

	return state;
};

/*
useGraphForPath answers which graph draws a given URL.

The path is not stored anywhere on this side: each graph carries its own
ui.UIRoute whose path input says what it answers to, so finding the graph for a
URL means asking the hub what it holds and reading that node. This is what makes
a route a node rather than an entry in a table — a surface moves by editing the
node that declares it.
*/
export const useGraphForPath = (
	pathname: string,
): { name?: string; status: "loading" | "resolved" | "absent" } => {
	const [index, setIndex] = useState<Record<string, string> | null>(null);

	useEffect(() => {
		let live = true;

		fetchDefinitions()
			.then(async (names) => {
				const routes: Record<string, string> = {};

				await Promise.all(
					names.map(async (name) => {
						const graph = await fetchDefinition(name).catch(() => null);
						const nodes = (graph?.nodes ?? {}) as Record<string, FlumeNode>;

						for (const node of Object.values(nodes)) {
							if ((node as { type?: string }).type !== "ui.UIRoute") {
								continue;
							}

							const data = (node as { inputData?: Record<string, any> })
								.inputData;
							const declared = data?.path?.value ?? data?.path;

							if (typeof declared === "string" && declared.length > 0) {
								routes[declared] = name;
							}
						}
					}),
				);

				if (live) {
					setIndex(routes);
				}
			})
			.catch(() => {
				if (live) {
					setIndex({});
				}
			});

		return () => {
			live = false;
		};
	}, []);

	if (index === null) {
		return { status: "loading" };
	}

	const name = index[pathname] ?? index[pathname.replace(/\/$/, "")];

	return name ? { name, status: "resolved" } : { status: "absent" };
};

/*
GraphSurface compiles one graph the hub holds and draws it.
*/
export const GraphSurface = ({ name }: { name: string }) => {
	const graph = useHubGraph(name);
	const nodes = graph.status === "loaded" ? graph.nodes : undefined;
	const results = useGraphResults(name, JSON.stringify(nodes ?? {}));
	const sources = useLiveSources();

	const compilation = useMemo(() => {
		if (!nodes || Object.keys(nodes).length === 0) {
			return null;
		}

		return compileUI({ nodes });
	}, [nodes]);

	if (graph.status === "loading") {
		return (
			<Flex.Center className="h-full w-full gap-2 text-(--f3)">
				<Spinner />
				<Typography.Mono>reading {name}</Typography.Mono>
			</Flex.Center>
		);
	}

	if (graph.status === "failed") {
		return (
			<Flex.Column className="h-full w-full p-4">
				<Alert variant="error">
					Cannot draw "{name}": {graph.reason}
				</Alert>
			</Flex.Column>
		);
	}

	if (!compilation || compilation.routes.length === 0) {
		return (
			<Flex.Column className="h-full w-full gap-2 p-4">
				<Alert variant="warning">
					"{name}" compiled, but it holds no ui.UIRoute and no top-level UI
					component, so there is nothing here to draw.
				</Alert>
			</Flex.Column>
		);
	}

	return (
		<div className="relative h-full w-full">
			{compilation.diagnostics.length > 0 && (
				<div className="absolute top-2 right-2 z-50 max-w-md">
					<Alert variant="warning">
						<Typography.Label>
							{compilation.diagnostics.length} compilation warning(s)
						</Typography.Label>
						<ul className="list-disc space-y-1 pl-4">
							{compilation.diagnostics.map((diagnostic, index) => (
								<li
									key={`${diagnostic.kind}-${diagnostic.nodeId ?? index}-${diagnostic.propName ?? ""}`}
								>
									{diagnostic.message}
								</li>
							))}
						</ul>
					</Alert>
				</div>
			)}
			{renderUIRoute(compilation.routes[0], results, sources)}
		</div>
	);
};

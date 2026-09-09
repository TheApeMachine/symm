import * as Comlink from "comlink";
import React from "react";
import type { EdgeRoutingMode } from "#/components/flume/connectionCalculator";
import {
	getStageRef,
	syncConnectionElements,
} from "#/components/flume/connectionCalculator";
import type { FlumeGraphWorkerHandle } from "#/components/flume/context";
import {
	portLayoutKey,
	type SpatialIndexSnapshot,
} from "#/components/flume/spatial-index";
import type {
	Coordinate,
	NodeMap,
	TransputType,
} from "#/components/flume/types";
import type { ConnectionPathResult } from "#/workers/flume-graph.types";
import type { FlumeGraphWorkerApi } from "#/workers/flume-graph.worker";

export type { FlumeGraphWorkerHandle };

/*
The worker owns the entire graph computation model. The main thread is
input-only: it pushes events (graph, port/node layout, routing mode,
drag) and pulls rendered output (roster + paths) on demand.

State changes auto-batch: any setter schedules a render request via
requestAnimationFrame so multiple updates in the same frame coalesce
into one round trip.
*/

const applyPathsToDOM = (
	paths: ReadonlyArray<ConnectionPathResult>,
	cache: Map<string, SVGPathElement>,
): void => {
	for (const { id, d } of paths) {
		let path = cache.get(id);

		if (!path || !path.isConnected) {
			path =
				document.querySelector<SVGPathElement>(
					`[data-connection-id="${id}"]`,
				) ?? undefined;

			if (path) {
				cache.set(id, path);
			} else {
				cache.delete(id);
			}
		}

		path?.setAttribute("d", d);
	}
};

export const useFlumeGraphWorker = (
	editorId: string,
	routingMode: EdgeRoutingMode,
	_indexRef: React.RefObject<SpatialIndexSnapshot>,
): FlumeGraphWorkerHandle => {
	const workerRef = React.useRef<Worker | null>(null);
	const apiRef = React.useRef<Comlink.Remote<FlumeGraphWorkerApi> | null>(null);
	const pathCacheRef = React.useRef<Map<string, SVGPathElement>>(new Map());
	const routingModeRef = React.useRef(routingMode);
	const renderFrameRef = React.useRef<number | null>(null);
	const renderingRef = React.useRef(false);
	const renderDirtyRef = React.useRef(false);
	const lastNodesRef = React.useRef<NodeMap | null>(null);

	routingModeRef.current = routingMode;

	React.useEffect(() => {
		const worker = new Worker(
			new URL("../../workers/flume-graph.worker.ts", import.meta.url),
			{ type: "module" },
		);
		const api = Comlink.wrap<FlumeGraphWorkerApi>(worker);

		workerRef.current = worker;
		apiRef.current = api;

		void api.setRoutingMode(routingModeRef.current);

		return () => {
			if (renderFrameRef.current !== null) {
				cancelAnimationFrame(renderFrameRef.current);
				renderFrameRef.current = null;
			}

			api[Comlink.releaseProxy]();
			worker.terminate();
			workerRef.current = null;
			apiRef.current = null;
			pathCacheRef.current.clear();
		};
	}, []);

	const performRenderRef = React.useRef<() => Promise<void>>(() =>
		Promise.resolve(),
	);
	const scheduleRenderRef = React.useRef<(() => void) | null>(null);

	const scheduleRender = React.useCallback(() => {
		if (renderingRef.current) {
			renderDirtyRef.current = true;
			return;
		}

		if (renderFrameRef.current !== null) return;

		renderFrameRef.current = requestAnimationFrame(() => {
			renderFrameRef.current = null;
			void performRenderRef.current();
		});
	}, []);

	const performRender = React.useCallback(async () => {
		const api = apiRef.current;
		if (!api) return;

		renderingRef.current = true;
		renderDirtyRef.current = false;

		try {
			const { paths, roster } = await api.recalculate();
			syncConnectionElements(roster, editorId, routingModeRef.current);
			applyPathsToDOM(paths, pathCacheRef.current);
		} finally {
			renderingRef.current = false;

			if (renderDirtyRef.current) {
				renderDirtyRef.current = false;
				scheduleRenderRef.current?.();
			}
		}
	}, [editorId]);

	performRenderRef.current = performRender;
	scheduleRenderRef.current = scheduleRender;

	const setGraph = React.useCallback(
		(nodes: NodeMap) => {
			const api = apiRef.current;
			if (!api) return;

			// useLiveQuery hands back proxy-wrapped objects from the
			// collection's reactive store. structuredClone (which
			// postMessage uses internally) can't transfer those proxies,
			// so JSON-roundtrip into a plain POJO before crossing the
			// worker boundary. Functions and Symbols get stripped — the
			// graph data is supposed to be pure data already.
			const plain = JSON.parse(JSON.stringify(nodes)) as NodeMap;

			lastNodesRef.current = plain;
			void (async () => {
				await api.setGraph(plain);
				scheduleRender();
			})();
		},
		[scheduleRender],
	);

	const setPortLayout = React.useCallback(
		(
			nodeId: string,
			portName: string,
			transputType: TransputType,
			offsetX: number,
			offsetY: number,
		) => {
			const api = apiRef.current;
			if (!api) return;

			void api.setPortLayout(nodeId, portName, transputType, offsetX, offsetY);
			scheduleRender();
		},
		[scheduleRender],
	);

	const setNodeLayout = React.useCallback(
		(nodeId: string, width: number, height: number) => {
			const api = apiRef.current;
			if (!api) return;

			void api.setNodeLayout(nodeId, width, height);
			scheduleRender();
		},
		[scheduleRender],
	);

	React.useEffect(() => {
		const api = apiRef.current;
		if (!api) return;

		void api.setRoutingMode(routingMode);
		scheduleRender();
	}, [routingMode, scheduleRender]);

	const beginDrag = React.useCallback((nodeId: string) => {
		const api = apiRef.current;
		if (!api) return;
		void api.beginDrag(nodeId);
	}, []);

	const updateDrag = React.useCallback(
		(nodeId: string, x: number, y: number) => {
			const api = apiRef.current;
			if (!api) return;
			void api.updateDrag(nodeId, x, y);
			scheduleRender();
		},
		[scheduleRender],
	);

	const endDrag = React.useCallback(
		(nodeId: string, x: number, y: number) => {
			const api = apiRef.current;
			if (!api) return;
			void api.endDrag(nodeId, x, y);
			scheduleRender();
		},
		[scheduleRender],
	);

	const recalculate = React.useCallback(
		(_nodes: NodeMap, _positionOverrides?: Record<string, Coordinate>) => {
			// Compatibility shim: NodeEditor's existing recalculate call
			// now just nudges the render pipeline. setGraph is the explicit
			// channel for topology updates.
			scheduleRender();
		},
		[scheduleRender],
	);

	return React.useMemo(
		() => ({
			beginDrag,
			updateDrag,
			endDrag,
			recalculate,
			setGraph,
			setPortLayout,
			setNodeLayout,
			scheduleRender,
		}),
		[
			beginDrag,
			endDrag,
			recalculate,
			scheduleRender,
			setGraph,
			setNodeLayout,
			setPortLayout,
			updateDrag,
		],
	);
};

// Suppress unused-warning for getStageRef import — kept available for the
// syncConnectionElements path inside the same module via re-export below.
export const _stageRef = getStageRef;

export { portLayoutKey };

import * as THREE from "three";
import type { Graph } from "./core/graph";
import { generateLayeredLayoutTexture as createLayeredLayoutTexture } from "./textures/layered";

export type LayoutType =
	| "force"
	| "circular"
	| "spherical"
	| "helix"
	| "grid"
	| "layered"
	| "cylinder"
	| "radialLayered"
	| "bfs"
	| "radialBfs"
	| "tree"
	| "dag"
	| "bfs3d"
	| "tree3d"
	| "dag3d"
	| "guided3d"
	| "grid3d";

export function normalizeLayout(
	value: string | null,
	fallback: LayoutType,
): LayoutType {
	if (!value) return fallback;

	// Migrate legacy/non-3D layouts to 3D equivalents so old saved values don’t “disappear”.
	switch (value) {
		case "bfs":
			return "bfs3d";
		case "tree":
			return "tree3d";
		case "dag":
			return "dag3d";
		case "layered":
			return "cylinder";
		case "circular":
			return "spherical";
		case "grid":
			return "grid3d";
	}

	// Allow current supported values.
	if (
		value === "force" ||
		value === "spherical" ||
		value === "helix" ||
		value === "cylinder" ||
		value === "radialLayered" ||
		value === "radialBfs" ||
		value === "bfs3d" ||
		value === "tree3d" ||
		value === "dag3d" ||
		value === "guided3d" ||
		value === "grid3d"
	) {
		return value;
	}

	return fallback;
}

type MinMax = {
	x: number;
	y: number;
	z: number;
};

type LayoutTextureFactoryOptions = {
	getGraph: () => Graph | null;
	setLayoutBoundsFromMinMax: (min: MinMax, max: MinMax) => void;
};

export function createLayoutTextureGenerators({
	getGraph,
	setLayoutBoundsFromMinMax,
}: LayoutTextureFactoryOptions) {
	const parseLayerToken = (
		nodeName: string,
	): { layer: number; token: number } | null => {
		// Attention visualizer node ids: `L{layerIndex}:{tokenIndex}:{token}`
		const m = /^L(\d+):(\d+):/.exec(nodeName);
		if (!m) return null;
		const layer = Number(m[1]);
		const token = Number(m[2]);
		if (!Number.isFinite(layer) || !Number.isFinite(token)) return null;
		return { layer, token };
	};

	const generateLayeredLayoutTexture = createLayeredLayoutTexture(
		parseLayerToken,
		setLayoutBoundsFromMinMax,
	);

	const generateCylinderLayoutTexture = (
		nodeNames: string[],
		nodesWidth: number,
	): THREE.DataTexture | null => {
		const parsed = nodeNames.map((n) => (n ? parseLayerToken(n) : null));
		const valid = parsed.filter(
			(x): x is { layer: number; token: number } => x !== null,
		);
		if (valid.length === 0) return null;

		let maxLayer = 0;
		let maxToken = 0;
		for (const { layer, token } of valid) {
			if (layer > maxLayer) maxLayer = layer;
			if (token > maxToken) maxToken = token;
		}

		const layerCount = maxLayer + 1;
		const tokenCount = maxToken + 1;

		const radius = 900;
		const layerSpacing = 260;
		const z0 = -((layerCount - 1) * layerSpacing) / 2;

		const textureArray = new Float32Array(nodesWidth * nodesWidth * 4);
		for (let i = 0; i < textureArray.length; i += 4) {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}

		for (let nodeIndex = 0; nodeIndex < nodeNames.length; nodeIndex++) {
			const p = parsed[nodeIndex];
			if (!p) continue;
			const angle = (p.token / Math.max(1, tokenCount)) * Math.PI * 2;
			const x = radius * Math.cos(angle);
			const y = radius * Math.sin(angle);
			const z = z0 + p.layer * layerSpacing;

			const base = nodeIndex * 4;
			textureArray[base] = x;
			textureArray[base + 1] = y;
			textureArray[base + 2] = z;
			textureArray[base + 3] = 1.0;
		}

		const texture = new THREE.DataTexture(
			textureArray,
			nodesWidth,
			nodesWidth,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	};

	const generateRadialLayeredLayoutTexture = (
		nodeNames: string[],
		nodesWidth: number,
	): THREE.DataTexture | null => {
		const parsed = nodeNames.map((n) => (n ? parseLayerToken(n) : null));
		const valid = parsed.filter(
			(x): x is { layer: number; token: number } => x !== null,
		);
		if (valid.length === 0) return null;

		let maxLayer = 0;
		let maxToken = 0;
		for (const { layer, token } of valid) {
			if (layer > maxLayer) maxLayer = layer;
			if (token > maxToken) maxToken = token;
		}

		const layerCount = maxLayer + 1;
		const tokenCount = maxToken + 1;

		const r0 = 250;
		const dr = 140;

		const textureArray = new Float32Array(nodesWidth * nodesWidth * 4);
		const min = {
			x: Number.POSITIVE_INFINITY,
			y: Number.POSITIVE_INFINITY,
			z: Number.POSITIVE_INFINITY,
		};
		const max = {
			x: Number.NEGATIVE_INFINITY,
			y: Number.NEGATIVE_INFINITY,
			z: Number.NEGATIVE_INFINITY,
		};
		for (let i = 0; i < textureArray.length; i += 4) {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}

		for (let nodeIndex = 0; nodeIndex < nodeNames.length; nodeIndex++) {
			const p = parsed[nodeIndex];
			if (!p) continue;
			const radius = r0 + p.layer * dr;
			const angle = (p.token / Math.max(1, tokenCount)) * Math.PI * 2;

			const x = radius * Math.cos(angle);
			const y = radius * Math.sin(angle);
			// Small z separation by layer keeps labels from z-fighting in some views.
			const z = (p.layer - (layerCount - 1) / 2) * 4;

			const base = nodeIndex * 4;
			textureArray[base] = x;
			textureArray[base + 1] = y;
			textureArray[base + 2] = z;
			textureArray[base + 3] = 1.0;

			if (x < min.x) min.x = x;
			if (y < min.y) min.y = y;
			if (z < min.z) min.z = z;
			if (x > max.x) max.x = x;
			if (y > max.y) max.y = y;
			if (z > max.z) max.z = z;
		}

		setLayoutBoundsFromMinMax(min, max);
		const texture = new THREE.DataTexture(
			textureArray,
			nodesWidth,
			nodesWidth,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	};

	const generateBfsLayoutTexture = (
		nodeNames: string[],
		nodesWidth: number,
	): THREE.DataTexture | null => {
		const g = getGraph();
		if (!g) return null;

		const nodeCount = nodeNames.length;
		if (nodeCount === 0) return null;

		const nodesById: Array<{ id: number; edges: number[] } | null> = new Array(
			nodeCount,
		).fill(null);
		for (const n of Object.values(g.nodes)) {
			if (n.id >= 0 && n.id < nodeCount) nodesById[n.id] = n;
		}

		// Build degree table to pick a good root (highest degree).
		const degrees: number[] = new Array(nodeCount).fill(0);
		for (const node of nodesById) {
			if (!node) continue;
			degrees[node.id] = node.edges.length;
		}
		let root = 0;
		for (let i = 1; i < nodeCount; i++) {
			if ((degrees[i] ?? 0) > (degrees[root] ?? 0)) root = i;
		}

		// BFS over the undirected adjacency already stored in the graph.
		const level: number[] = new Array(nodeCount).fill(-1);
		const queue: number[] = [];
		level[root] = 0;
		queue.push(root);
		while (queue.length > 0) {
			const u = queue.shift();
			if (u === undefined) break;
			const node = nodesById[u];
			if (!node) continue;
			for (const v of node.edges) {
				if (v < 0 || v >= nodeCount) continue;
				if (level[v] !== -1) continue;
				level[v] = (level[u] ?? 0) + 1;
				queue.push(v);
			}
		}

		// Unreached nodes go to the last level bucket.
		let maxLevel = 0;
		for (const l of level) if (l > maxLevel) maxLevel = l;
		const disconnectedLevel = maxLevel + 1;
		for (let i = 0; i < nodeCount; i++) {
			if (level[i] === -1) level[i] = disconnectedLevel;
		}
		maxLevel = Math.max(maxLevel, disconnectedLevel);

		const buckets: number[][] = new Array(maxLevel + 1).fill(0).map(() => []);
		for (let i = 0; i < nodeCount; i++) buckets[level[i] ?? 0]?.push(i);

		// Stable order within a layer (by id).
		for (const b of buckets) b.sort((a, b) => a - b);

		const xSpacing = 120;
		const ySpacing = 320;

		const textureArray = new Float32Array(nodesWidth * nodesWidth * 4);
		const min = {
			x: Number.POSITIVE_INFINITY,
			y: Number.POSITIVE_INFINITY,
			z: Number.POSITIVE_INFINITY,
		};
		const max = {
			x: Number.NEGATIVE_INFINITY,
			y: Number.NEGATIVE_INFINITY,
			z: Number.NEGATIVE_INFINITY,
		};
		for (let i = 0; i < textureArray.length; i += 4) {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}

		for (let l = 0; l < buckets.length; l++) {
			const ids = buckets[l] ?? [];
			const rowWidth = (ids.length - 1) * xSpacing;
			for (let j = 0; j < ids.length; j++) {
				const id = ids[j];
				if (id === undefined) continue;
				const x = j * xSpacing - rowWidth / 2;
				const y = -l * ySpacing + (maxLevel * ySpacing) / 2;
				const z = (l - maxLevel / 2) * 12;
				const base = id * 4;
				textureArray[base] = x;
				textureArray[base + 1] = y;
				textureArray[base + 2] = z;
				textureArray[base + 3] = 1.0;

				if (x < min.x) min.x = x;
				if (y < min.y) min.y = y;
				if (z < min.z) min.z = z;
				if (x > max.x) max.x = x;
				if (y > max.y) max.y = y;
				if (z > max.z) max.z = z;
			}
		}

		setLayoutBoundsFromMinMax(min, max);
		const texture = new THREE.DataTexture(
			textureArray,
			nodesWidth,
			nodesWidth,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	};

	const generateBfs3dLayoutTexture = (
		nodeNames: string[],
		nodesWidth: number,
	): THREE.DataTexture | null => {
		const g = getGraph();
		if (!g) return null;

		const nodeCount = nodeNames.length;
		if (nodeCount === 0) return null;

		const nodesById: Array<{ id: number; edges: number[] } | null> = new Array(
			nodeCount,
		).fill(null);
		for (const n of Object.values(g.nodes)) {
			if (n.id >= 0 && n.id < nodeCount) nodesById[n.id] = n;
		}

		// Root: highest degree.
		const degrees: number[] = new Array(nodeCount).fill(0);
		for (const node of nodesById) {
			if (!node) continue;
			degrees[node.id] = node.edges.length;
		}
		let root = 0;
		for (let i = 1; i < nodeCount; i++) {
			if ((degrees[i] ?? 0) > (degrees[root] ?? 0)) root = i;
		}

		const level: number[] = new Array(nodeCount).fill(-1);
		const queue: number[] = [];
		level[root] = 0;
		queue.push(root);
		while (queue.length > 0) {
			const u = queue.shift();
			if (u === undefined) break;
			const node = nodesById[u];
			if (!node) continue;
			for (const v of node.edges) {
				if (v < 0 || v >= nodeCount) continue;
				if (level[v] !== -1) continue;
				level[v] = (level[u] ?? 0) + 1;
				queue.push(v);
			}
		}

		let maxLevel = 0;
		for (const l of level) if (l > maxLevel) maxLevel = l;
		const disconnectedLevel = maxLevel + 1;
		for (let i = 0; i < nodeCount; i++) {
			if (level[i] === -1) level[i] = disconnectedLevel;
		}
		maxLevel = Math.max(maxLevel, disconnectedLevel);

		const buckets: number[][] = new Array(maxLevel + 1).fill(0).map(() => []);
		for (let i = 0; i < nodeCount; i++) buckets[level[i] ?? 0]?.push(i);
		for (const b of buckets) b.sort((a, b) => a - b);

		const xSpacing = 160;
		const zSpacing = 160;
		const ySpacing = 360;

		const textureArray = new Float32Array(nodesWidth * nodesWidth * 4);
		const min = {
			x: Number.POSITIVE_INFINITY,
			y: Number.POSITIVE_INFINITY,
			z: Number.POSITIVE_INFINITY,
		};
		const max = {
			x: Number.NEGATIVE_INFINITY,
			y: Number.NEGATIVE_INFINITY,
			z: Number.NEGATIVE_INFINITY,
		};
		for (let i = 0; i < textureArray.length; i += 4) {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}

		for (let l = 0; l < buckets.length; l++) {
			const ids = buckets[l] ?? [];
			const cols = Math.max(1, Math.ceil(Math.sqrt(ids.length)));
			const rows = Math.max(1, Math.ceil(ids.length / cols));
			const x0 = -((cols - 1) * xSpacing) / 2;
			const z0 = -((rows - 1) * zSpacing) / 2;
			const y = -l * ySpacing + (maxLevel * ySpacing) / 2;

			for (let j = 0; j < ids.length; j++) {
				const id = ids[j];
				if (id === undefined) continue;
				const col = j % cols;
				const row = Math.floor(j / cols);
				const x = x0 + col * xSpacing;
				const z = z0 + row * zSpacing;

				const base = id * 4;
				textureArray[base] = x;
				textureArray[base + 1] = y;
				textureArray[base + 2] = z;
				textureArray[base + 3] = 1.0;

				if (x < min.x) min.x = x;
				if (y < min.y) min.y = y;
				if (z < min.z) min.z = z;
				if (x > max.x) max.x = x;
				if (y > max.y) max.y = y;
				if (z > max.z) max.z = z;
			}
		}

		setLayoutBoundsFromMinMax(min, max);
		const texture = new THREE.DataTexture(
			textureArray,
			nodesWidth,
			nodesWidth,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	};

	const generateRadialBfsLayoutTexture = (
		nodeNames: string[],
		nodesWidth: number,
	): THREE.DataTexture | null => {
		const g = getGraph();
		if (!g) return null;

		const nodeCount = nodeNames.length;
		if (nodeCount === 0) return null;

		const nodesById: Array<{ id: number; edges: number[] } | null> = new Array(
			nodeCount,
		).fill(null);
		for (const n of Object.values(g.nodes)) {
			if (n.id >= 0 && n.id < nodeCount) nodesById[n.id] = n;
		}

		// Choose a stable root: highest degree (good for architecture graphs).
		const degrees: number[] = new Array(nodeCount).fill(0);
		for (const node of nodesById) {
			if (!node) continue;
			degrees[node.id] = node.edges.length;
		}
		let root = 0;
		for (let i = 1; i < nodeCount; i++) {
			if ((degrees[i] ?? 0) > (degrees[root] ?? 0)) root = i;
		}

		// BFS levels.
		const level: number[] = new Array(nodeCount).fill(-1);
		const queue: number[] = [];
		level[root] = 0;
		queue.push(root);
		while (queue.length > 0) {
			const u = queue.shift();
			if (u === undefined) break;
			const node = nodesById[u];
			if (!node) continue;
			for (const v of node.edges) {
				if (v < 0 || v >= nodeCount) continue;
				if (level[v] !== -1) continue;
				level[v] = (level[u] ?? 0) + 1;
				queue.push(v);
			}
		}

		let maxLevel = 0;
		for (const l of level) if (l > maxLevel) maxLevel = l;
		const disconnectedLevel = maxLevel + 1;
		for (let i = 0; i < nodeCount; i++) {
			if (level[i] === -1) level[i] = disconnectedLevel;
		}
		maxLevel = Math.max(maxLevel, disconnectedLevel);

		const rings: number[][] = new Array(maxLevel + 1).fill(0).map(() => []);
		for (let i = 0; i < nodeCount; i++) rings[level[i] ?? 0]?.push(i);
		for (const r of rings) r.sort((a, b) => a - b);

		const r0 = 120;
		const dr = 220;

		const textureArray = new Float32Array(nodesWidth * nodesWidth * 4);
		const min = {
			x: Number.POSITIVE_INFINITY,
			y: Number.POSITIVE_INFINITY,
			z: Number.POSITIVE_INFINITY,
		};
		const max = {
			x: Number.NEGATIVE_INFINITY,
			y: Number.NEGATIVE_INFINITY,
			z: Number.NEGATIVE_INFINITY,
		};
		for (let i = 0; i < textureArray.length; i += 4) {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}

		for (let l = 0; l < rings.length; l++) {
			const ids = rings[l] ?? [];
			const radius = r0 + l * dr;
			const denom = Math.max(1, ids.length);
			for (let j = 0; j < ids.length; j++) {
				const id = ids[j];
				if (id === undefined) continue;
				const angle = (j / denom) * Math.PI * 2;
				const x = radius * Math.cos(angle);
				const y = radius * Math.sin(angle);
				const z = 0;
				const base = id * 4;
				textureArray[base] = x;
				textureArray[base + 1] = y;
				textureArray[base + 2] = z;
				textureArray[base + 3] = 1.0;

				if (x < min.x) min.x = x;
				if (y < min.y) min.y = y;
				if (z < min.z) min.z = z;
				if (x > max.x) max.x = x;
				if (y > max.y) max.y = y;
				if (z > max.z) max.z = z;
			}
		}

		setLayoutBoundsFromMinMax(min, max);
		const texture = new THREE.DataTexture(
			textureArray,
			nodesWidth,
			nodesWidth,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	};

	const generateSpanningTreeLayoutTexture = (
		nodeNames: string[],
		nodesWidth: number,
	): THREE.DataTexture | null => {
		const g = getGraph();
		if (!g) return null;

		const nodeCount = nodeNames.length;
		if (nodeCount === 0) return null;

		const nodesById: Array<{ id: number; edges: number[] } | null> = new Array(
			nodeCount,
		).fill(null);
		for (const n of Object.values(g.nodes)) {
			if (n.id >= 0 && n.id < nodeCount) nodesById[n.id] = n;
		}

		// Root = highest degree.
		const degrees: number[] = new Array(nodeCount).fill(0);
		for (const node of nodesById) {
			if (!node) continue;
			degrees[node.id] = node.edges.length;
		}
		let root = 0;
		for (let i = 1; i < nodeCount; i++) {
			if ((degrees[i] ?? 0) > (degrees[root] ?? 0)) root = i;
		}

		// Build BFS parent pointers (spanning forest).
		const parent: number[] = new Array(nodeCount).fill(-1);
		const depth: number[] = new Array(nodeCount).fill(-1);
		const queue: number[] = [];
		depth[root] = 0;
		queue.push(root);

		while (queue.length > 0) {
			const u = queue.shift();
			if (u === undefined) break;
			const node = nodesById[u];
			if (!node) continue;
			for (const v of node.edges) {
				if (v < 0 || v >= nodeCount) continue;
				if (depth[v] !== -1) continue;
				parent[v] = u;
				depth[v] = (depth[u] ?? 0) + 1;
				queue.push(v);
			}
		}

		// Attach disconnected components as extra roots under a virtual level.
		let maxDepth = 0;
		for (const d of depth) if (d > maxDepth) maxDepth = d;
		const disconnectedDepth = maxDepth + 1;
		for (let i = 0; i < nodeCount; i++) {
			if (depth[i] === -1) {
				parent[i] = -1;
				depth[i] = disconnectedDepth;
			}
		}
		maxDepth = Math.max(maxDepth, disconnectedDepth);

		const children: number[][] = new Array(nodeCount).fill(0).map(() => []);
		for (let i = 0; i < nodeCount; i++) {
			const p = parent[i];
			if (p >= 0) children[p]?.push(i);
		}
		for (const ch of children) ch.sort((a, b) => a - b);

		// Subtree sizes + x coordinates via DFS ordering.
		const subtree: number[] = new Array(nodeCount).fill(1);
		const xPos: number[] = new Array(nodeCount).fill(0);

		const computeSubtree = (u: number): number => {
			const ch = children[u] ?? [];
			let size = 1;
			for (const v of ch) size += computeSubtree(v);
			subtree[u] = size;
			return size;
		};

		// Identify top-level roots (including the main root + any disconnected nodes with parent=-1).
		const roots: number[] = [];
		for (let i = 0; i < nodeCount; i++) {
			if (i === root) continue;
			if (parent[i] === -1 && i !== root) roots.push(i);
		}
		roots.sort((a, b) => a - b);
		const forestRoots = [root, ...roots];

		for (const r of forestRoots) {
			if (r >= 0 && r < nodeCount) computeSubtree(r);
		}

		let cursor = 0;
		const assignX = (u: number) => {
			const ch = children[u] ?? [];
			if (ch.length === 0) {
				xPos[u] = cursor;
				cursor += 1;
				return;
			}
			for (const v of ch) assignX(v);
			const first = ch[0];
			const last = ch[ch.length - 1];
			if (first === undefined || last === undefined) return;
			xPos[u] = (xPos[first] + xPos[last]) / 2;
		};

		// Lay out each tree in the forest side-by-side.
		cursor = 0;
		for (const r of forestRoots) {
			const start = cursor;
			assignX(r);
			const end = cursor;
			// Add a gap between components.
			cursor = end + 2;
			// Center this component around its local span.
			const mid = (start + end) / 2;
			const shift = xPos[r] - mid;
			// Shift all nodes in this component by traversing it again.
			const stack: number[] = [r];
			while (stack.length) {
				const u = stack.pop();
				if (u === undefined) break;
				xPos[u] -= shift;
				for (const v of children[u] ?? []) stack.push(v);
			}
		}

		const xSpacing = 140;
		const ySpacing = 260;
		const y0 = (maxDepth * ySpacing) / 2;
		const x0 = (cursor * xSpacing) / 2;

		const textureArray = new Float32Array(nodesWidth * nodesWidth * 4);
		const min = {
			x: Number.POSITIVE_INFINITY,
			y: Number.POSITIVE_INFINITY,
			z: Number.POSITIVE_INFINITY,
		};
		const max = {
			x: Number.NEGATIVE_INFINITY,
			y: Number.NEGATIVE_INFINITY,
			z: Number.NEGATIVE_INFINITY,
		};
		for (let i = 0; i < textureArray.length; i += 4) {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}

		for (let i = 0; i < nodeCount; i++) {
			const d = depth[i];
			if (d < 0) continue;
			const x = xPos[i] * xSpacing - x0;
			const y = -d * ySpacing + y0;
			const z = (d - maxDepth / 2) * 12;
			const base = i * 4;
			textureArray[base] = x;
			textureArray[base + 1] = y;
			textureArray[base + 2] = z;
			textureArray[base + 3] = 1.0;

			if (x < min.x) min.x = x;
			if (y < min.y) min.y = y;
			if (z < min.z) min.z = z;
			if (x > max.x) max.x = x;
			if (y > max.y) max.y = y;
			if (z > max.z) max.z = z;
		}

		setLayoutBoundsFromMinMax(min, max);
		const texture = new THREE.DataTexture(
			textureArray,
			nodesWidth,
			nodesWidth,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	};

	const generateTree3dLayoutTexture = (
		nodeNames: string[],
		nodesWidth: number,
	): THREE.DataTexture | null => {
		const g = getGraph();
		if (!g) return null;

		const nodeCount = nodeNames.length;
		if (nodeCount === 0) return null;

		const nodesById: Array<{ id: number; edges: number[] } | null> = new Array(
			nodeCount,
		).fill(null);
		for (const n of Object.values(g.nodes)) {
			if (n.id >= 0 && n.id < nodeCount) nodesById[n.id] = n;
		}

		// Root: highest degree.
		const degrees: number[] = new Array(nodeCount).fill(0);
		for (const node of nodesById) {
			if (!node) continue;
			degrees[node.id] = node.edges.length;
		}
		let root = 0;
		for (let i = 1; i < nodeCount; i++) {
			if ((degrees[i] ?? 0) > (degrees[root] ?? 0)) root = i;
		}

		// BFS parent tree.
		const parent: number[] = new Array(nodeCount).fill(-1);
		const depth: number[] = new Array(nodeCount).fill(-1);
		const queue: number[] = [];
		depth[root] = 0;
		queue.push(root);
		while (queue.length > 0) {
			const u = queue.shift();
			if (u === undefined) break;
			const node = nodesById[u];
			if (!node) continue;
			for (const v of node.edges) {
				if (v < 0 || v >= nodeCount) continue;
				if (depth[v] !== -1) continue;
				parent[v] = u;
				depth[v] = (depth[u] ?? 0) + 1;
				queue.push(v);
			}
		}

		let maxDepth = 0;
		for (const d of depth) if (d > maxDepth) maxDepth = d;
		const disconnectedDepth = maxDepth + 1;
		for (let i = 0; i < nodeCount; i++) {
			if (depth[i] === -1) depth[i] = disconnectedDepth;
		}
		maxDepth = Math.max(maxDepth, disconnectedDepth);

		const buckets: number[][] = new Array(maxDepth + 1).fill(0).map(() => []);
		for (let i = 0; i < nodeCount; i++) buckets[depth[i] ?? 0]?.push(i);
		for (const b of buckets) b.sort((a, b) => a - b);

		const xSpacing = 150;
		const zSpacing = 150;
		const ySpacing = 360;

		const textureArray = new Float32Array(nodesWidth * nodesWidth * 4);
		const min = {
			x: Number.POSITIVE_INFINITY,
			y: Number.POSITIVE_INFINITY,
			z: Number.POSITIVE_INFINITY,
		};
		const max = {
			x: Number.NEGATIVE_INFINITY,
			y: Number.NEGATIVE_INFINITY,
			z: Number.NEGATIVE_INFINITY,
		};
		for (let i = 0; i < textureArray.length; i += 4) {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}

		for (let l = 0; l < buckets.length; l++) {
			const ids = buckets[l] ?? [];
			const cols = Math.max(1, Math.ceil(Math.sqrt(ids.length)));
			const rows = Math.max(1, Math.ceil(ids.length / cols));
			const x0 = -((cols - 1) * xSpacing) / 2;
			const z0 = -((rows - 1) * zSpacing) / 2;
			const y = -l * ySpacing + (maxDepth * ySpacing) / 2;

			for (let j = 0; j < ids.length; j++) {
				const id = ids[j];
				if (id === undefined) continue;
				const col = j % cols;
				const row = Math.floor(j / cols);
				const x = x0 + col * xSpacing;
				const z = z0 + row * zSpacing;

				const base = id * 4;
				textureArray[base] = x;
				textureArray[base + 1] = y;
				textureArray[base + 2] = z;
				textureArray[base + 3] = 1.0;

				if (x < min.x) min.x = x;
				if (y < min.y) min.y = y;
				if (z < min.z) min.z = z;
				if (x > max.x) max.x = x;
				if (y > max.y) max.y = y;
				if (z > max.z) max.z = z;
			}
		}

		setLayoutBoundsFromMinMax(min, max);
		const texture = new THREE.DataTexture(
			textureArray,
			nodesWidth,
			nodesWidth,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	};

	const generateDagLayoutTexture = (
		nodeNames: string[],
		nodesWidth: number,
	): THREE.DataTexture | null => {
		const g = getGraph();
		if (!g) return null;

		const nodeCount = nodeNames.length;
		if (nodeCount === 0) return null;

		// Map name -> id
		const idByName: Record<string, number> = {};
		for (const [name, node] of Object.entries(g.nodes)) {
			idByName[name] = node.id;
		}

		// Build directed edge list from stored edge orientation.
		const outEdges: number[][] = new Array(nodeCount).fill(0).map(() => []);
		const inEdges: number[][] = new Array(nodeCount).fill(0).map(() => []);

		for (const e of Object.values(g.edges)) {
			const s = idByName[e.source];
			const t = idByName[e.target];
			if (typeof s !== "number" || typeof t !== "number") continue;
			if (s < 0 || s >= nodeCount || t < 0 || t >= nodeCount) continue;
			if (s === t) continue;
			outEdges[s].push(t);
			inEdges[t].push(s);
		}

		// Break cycles conservatively: during DFS, ignore back-edges.
		const visiting: boolean[] = new Array(nodeCount).fill(false);
		const visited: boolean[] = new Array(nodeCount).fill(false);
		const dagOut: number[][] = new Array(nodeCount).fill(0).map(() => []);
		const dagIn: number[][] = new Array(nodeCount).fill(0).map(() => []);

		const dfs = (u: number) => {
			visiting[u] = true;
			for (const v of outEdges[u] ?? []) {
				if (visiting[v]) continue; // drop back-edge
				dagOut[u].push(v);
				dagIn[v].push(u);
				if (!visited[v]) dfs(v);
			}
			visiting[u] = false;
			visited[u] = true;
		};

		// Prefer a stable set of roots: nodes with minimal indegree.
		const indeg = inEdges.map((xs) => xs.length);
		const roots = indeg
			.map((d, i) => ({ d, i }))
			.sort((a, b) => a.d - b.d || a.i - b.i)
			.slice(0, Math.max(1, Math.min(8, nodeCount)))
			.map((x) => x.i);

		for (const r of roots) {
			if (!visited[r]) dfs(r);
		}
		for (let i = 0; i < nodeCount; i++) {
			if (!visited[i]) dfs(i);
		}

		// Longest-path layering on the DAG (topological DP).
		const topo: number[] = [];
		const seen: boolean[] = new Array(nodeCount).fill(false);
		const stackVisiting: boolean[] = new Array(nodeCount).fill(false);
		const topoDfs = (u: number) => {
			if (seen[u]) return;
			if (stackVisiting[u]) return;
			stackVisiting[u] = true;
			for (const v of dagOut[u] ?? []) topoDfs(v);
			stackVisiting[u] = false;
			seen[u] = true;
			topo.push(u);
		};
		for (let i = 0; i < nodeCount; i++) topoDfs(i);
		topo.reverse();

		const layer: number[] = new Array(nodeCount).fill(0);
		for (const u of topo) {
			for (const v of dagOut[u] ?? []) {
				const cand = (layer[u] ?? 0) + 1;
				if (cand > (layer[v] ?? 0)) layer[v] = cand;
			}
		}
		let maxLayer = 0;
		for (const l of layer) if (l > maxLayer) maxLayer = l;

		// Initial ordering by id in each layer.
		const layers: number[][] = new Array(maxLayer + 1).fill(0).map(() => []);
		for (let i = 0; i < nodeCount; i++) layers[layer[i] ?? 0]?.push(i);
		for (const ls of layers) ls.sort((a, b) => a - b);

		const indexInLayer = (ls: number[]) => {
			const pos: number[] = new Array(nodeCount).fill(-1);
			for (let i = 0; i < ls.length; i++) {
				const id = ls[i];
				if (id === undefined) continue;
				pos[id] = i;
			}
			return pos;
		};

		const reorderByBarycenter = (
			upper: number[],
			lower: number[],
			dir: "down" | "up",
		) => {
			const upperPos = indexInLayer(upper);
			const scored = lower.map((n) => {
				const neighbors = dir === "down" ? (dagIn[n] ?? []) : (dagOut[n] ?? []);
				let sum = 0;
				let cnt = 0;
				for (const m of neighbors) {
					const p = upperPos[m];
					if (p >= 0) {
						sum += p;
						cnt++;
					}
				}
				const bary = cnt > 0 ? sum / cnt : Number.POSITIVE_INFINITY;
				return { bary, n };
			});
			scored.sort((a, b) => a.bary - b.bary || a.n - b.n);
			return scored.map((x) => x.n);
		};

		// Crossing reduction: a few sweeps.
		for (let iter = 0; iter < 6; iter++) {
			for (let l = 0; l < layers.length - 1; l++) {
				const upper = layers[l] ?? [];
				const lower = layers[l + 1] ?? [];
				layers[l + 1] = reorderByBarycenter(upper, lower, "down");
			}
			for (let l = layers.length - 1; l > 0; l--) {
				const upper = layers[l] ?? [];
				const lower = layers[l - 1] ?? [];
				layers[l - 1] = reorderByBarycenter(upper, lower, "up");
			}
		}

		// Positions
		const xSpacing = 130;
		const ySpacing = 320;
		const y0 = (maxLayer * ySpacing) / 2;

		const xPos: number[] = new Array(nodeCount).fill(0);
		for (let l = 0; l < layers.length; l++) {
			const ls = layers[l] ?? [];
			const rowWidth = (ls.length - 1) * xSpacing;
			for (let i = 0; i < ls.length; i++) {
				const id = ls[i];
				if (id === undefined) continue;
				xPos[id] = i * xSpacing - rowWidth / 2;
			}
		}

		const textureArray = new Float32Array(nodesWidth * nodesWidth * 4);
		const min = {
			x: Number.POSITIVE_INFINITY,
			y: Number.POSITIVE_INFINITY,
			z: Number.POSITIVE_INFINITY,
		};
		const max = {
			x: Number.NEGATIVE_INFINITY,
			y: Number.NEGATIVE_INFINITY,
			z: Number.NEGATIVE_INFINITY,
		};
		for (let i = 0; i < textureArray.length; i += 4) {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}

		for (let i = 0; i < nodeCount; i++) {
			const x = xPos[i] ?? 0;
			const y = -(layer[i] ?? 0) * ySpacing + y0;
			const z = ((layer[i] ?? 0) - maxLayer / 2) * 14;
			const base = i * 4;
			textureArray[base] = x;
			textureArray[base + 1] = y;
			textureArray[base + 2] = z;
			textureArray[base + 3] = 1.0;

			if (x < min.x) min.x = x;
			if (y < min.y) min.y = y;
			if (z < min.z) min.z = z;
			if (x > max.x) max.x = x;
			if (y > max.y) max.y = y;
			if (z > max.z) max.z = z;
		}

		setLayoutBoundsFromMinMax(min, max);
		const texture = new THREE.DataTexture(
			textureArray,
			nodesWidth,
			nodesWidth,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	};

	const generateDag3dLayoutTexture = (
		nodeNames: string[],
		nodesWidth: number,
	): THREE.DataTexture | null => {
		// Reuse DAG ordering/layering, but place each layer on an X–Z grid to occupy volume.
		const g = getGraph();
		if (!g) return null;

		const nodeCount = nodeNames.length;
		if (nodeCount === 0) return null;

		const idByName: Record<string, number> = {};
		for (const [name, node] of Object.entries(g.nodes))
			idByName[name] = node.id;

		const outEdges: number[][] = new Array(nodeCount).fill(0).map(() => []);
		const inEdges: number[][] = new Array(nodeCount).fill(0).map(() => []);
		for (const e of Object.values(g.edges)) {
			const s = idByName[e.source];
			const t = idByName[e.target];
			if (typeof s !== "number" || typeof t !== "number") continue;
			if (s < 0 || s >= nodeCount || t < 0 || t >= nodeCount) continue;
			if (s === t) continue;
			outEdges[s].push(t);
			inEdges[t].push(s);
		}

		// Drop back-edges to form a DAG backbone.
		const visiting: boolean[] = new Array(nodeCount).fill(false);
		const visited: boolean[] = new Array(nodeCount).fill(false);
		const dagOut: number[][] = new Array(nodeCount).fill(0).map(() => []);
		const dagIn: number[][] = new Array(nodeCount).fill(0).map(() => []);
		const dfs = (u: number) => {
			visiting[u] = true;
			for (const v of outEdges[u] ?? []) {
				if (visiting[v]) continue;
				dagOut[u].push(v);
				dagIn[v].push(u);
				if (!visited[v]) dfs(v);
			}
			visiting[u] = false;
			visited[u] = true;
		};

		const indeg = inEdges.map((xs) => xs.length);
		const roots = indeg
			.map((d, i) => ({ d, i }))
			.sort((a, b) => a.d - b.d || a.i - b.i)
			.slice(0, Math.max(1, Math.min(8, nodeCount)))
			.map((x) => x.i);
		for (const r of roots) if (!visited[r]) dfs(r);
		for (let i = 0; i < nodeCount; i++) if (!visited[i]) dfs(i);

		// Topological order via DFS on dagOut.
		const topo: number[] = [];
		const seen: boolean[] = new Array(nodeCount).fill(false);
		const stackVis: boolean[] = new Array(nodeCount).fill(false);
		const topoDfs = (u: number) => {
			if (seen[u]) return;
			if (stackVis[u]) return;
			stackVis[u] = true;
			for (const v of dagOut[u] ?? []) topoDfs(v);
			stackVis[u] = false;
			seen[u] = true;
			topo.push(u);
		};
		for (let i = 0; i < nodeCount; i++) topoDfs(i);
		topo.reverse();

		const layer: number[] = new Array(nodeCount).fill(0);
		for (const u of topo) {
			for (const v of dagOut[u] ?? []) {
				const cand = (layer[u] ?? 0) + 1;
				if (cand > (layer[v] ?? 0)) layer[v] = cand;
			}
		}
		let maxLayer = 0;
		for (const l of layer) if (l > maxLayer) maxLayer = l;

		const layers: number[][] = new Array(maxLayer + 1).fill(0).map(() => []);
		for (let i = 0; i < nodeCount; i++) layers[layer[i] ?? 0]?.push(i);
		for (const ls of layers) ls.sort((a, b) => a - b);

		// A few barycenter sweeps for better adjacency grouping.
		const indexInLayer = (ls: number[]) => {
			const pos: number[] = new Array(nodeCount).fill(-1);
			for (let i = 0; i < ls.length; i++) {
				const id = ls[i];
				if (id === undefined) continue;
				pos[id] = i;
			}
			return pos;
		};
		const reorderByBarycenter = (
			upper: number[],
			lower: number[],
			dir: "down" | "up",
		) => {
			const upperPos = indexInLayer(upper);
			const scored = lower.map((n) => {
				const neighbors = dir === "down" ? (dagIn[n] ?? []) : (dagOut[n] ?? []);
				let sum = 0;
				let cnt = 0;
				for (const m of neighbors) {
					const p = upperPos[m];
					if (p >= 0) {
						sum += p;
						cnt++;
					}
				}
				const bary = cnt > 0 ? sum / cnt : Number.POSITIVE_INFINITY;
				return { bary, n };
			});
			scored.sort((a, b) => a.bary - b.bary || a.n - b.n);
			return scored.map((x) => x.n);
		};
		for (let iter = 0; iter < 6; iter++) {
			for (let l = 0; l < layers.length - 1; l++) {
				const upper = layers[l] ?? [];
				const lower = layers[l + 1] ?? [];
				layers[l + 1] = reorderByBarycenter(upper, lower, "down");
			}
			for (let l = layers.length - 1; l > 0; l--) {
				const upper = layers[l] ?? [];
				const lower = layers[l - 1] ?? [];
				layers[l - 1] = reorderByBarycenter(upper, lower, "up");
			}
		}

		const xSpacing = 150;
		const zSpacing = 150;
		const ySpacing = 360;

		const textureArray = new Float32Array(nodesWidth * nodesWidth * 4);
		const min = {
			x: Number.POSITIVE_INFINITY,
			y: Number.POSITIVE_INFINITY,
			z: Number.POSITIVE_INFINITY,
		};
		const max = {
			x: Number.NEGATIVE_INFINITY,
			y: Number.NEGATIVE_INFINITY,
			z: Number.NEGATIVE_INFINITY,
		};
		for (let i = 0; i < textureArray.length; i += 4) {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}

		for (let l = 0; l < layers.length; l++) {
			const ids = layers[l] ?? [];
			const cols = Math.max(1, Math.ceil(Math.sqrt(ids.length)));
			const rows = Math.max(1, Math.ceil(ids.length / cols));
			const x0 = -((cols - 1) * xSpacing) / 2;
			const z0 = -((rows - 1) * zSpacing) / 2;
			const y = -l * ySpacing + (maxLayer * ySpacing) / 2;

			for (let j = 0; j < ids.length; j++) {
				const id = ids[j];
				if (id === undefined) continue;
				const col = j % cols;
				const row = Math.floor(j / cols);
				const x = x0 + col * xSpacing;
				const z = z0 + row * zSpacing;

				const base = id * 4;
				textureArray[base] = x;
				textureArray[base + 1] = y;
				textureArray[base + 2] = z;
				textureArray[base + 3] = 1.0;

				if (x < min.x) min.x = x;
				if (y < min.y) min.y = y;
				if (z < min.z) min.z = z;
				if (x > max.x) max.x = x;
				if (y > max.y) max.y = y;
				if (z > max.z) max.z = z;
			}
		}

		setLayoutBoundsFromMinMax(min, max);
		const texture = new THREE.DataTexture(
			textureArray,
			nodesWidth,
			nodesWidth,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	};

	const generateGuided3dLayoutTexture = (
		nodeNames: string[],
		nodesWidth: number,
	): THREE.DataTexture | null => {
		// Plane-constraint texture: sets ONLY the Y target based on BFS depth.
		// X/Z are left at 0 and ignored by layoutMask in hybrid mode.
		const g = getGraph();
		if (!g) return null;

		const nodeCount = nodeNames.length;
		if (nodeCount === 0) return null;

		const nodesById: Array<{ id: number; edges: number[] } | null> = new Array(
			nodeCount,
		).fill(null);
		for (const n of Object.values(g.nodes)) {
			if (n.id >= 0 && n.id < nodeCount) nodesById[n.id] = n;
		}

		// Root: highest degree.
		const degrees: number[] = new Array(nodeCount).fill(0);
		for (const node of nodesById) {
			if (!node) continue;
			degrees[node.id] = node.edges.length;
		}
		let root = 0;
		for (let i = 1; i < nodeCount; i++) {
			if ((degrees[i] ?? 0) > (degrees[root] ?? 0)) root = i;
		}

		const level: number[] = new Array(nodeCount).fill(-1);
		const queue: number[] = [];
		level[root] = 0;
		queue.push(root);
		while (queue.length > 0) {
			const u = queue.shift();
			if (u === undefined) break;
			const node = nodesById[u];
			if (!node) continue;
			for (const v of node.edges) {
				if (v < 0 || v >= nodeCount) continue;
				if (level[v] !== -1) continue;
				level[v] = (level[u] ?? 0) + 1;
				queue.push(v);
			}
		}

		let maxLevel = 0;
		for (const l of level) if (l > maxLevel) maxLevel = l;
		const disconnectedLevel = maxLevel + 1;
		for (let i = 0; i < nodeCount; i++) {
			if (level[i] === -1) level[i] = disconnectedLevel;
		}
		maxLevel = Math.max(maxLevel, disconnectedLevel);

		const ySpacing = 360;
		const y0 = (maxLevel * ySpacing) / 2;

		const textureArray = new Float32Array(nodesWidth * nodesWidth * 4);
		// Conservative bounds for camera fit: X/Z will spread via force sim.
		const min = { x: -1200, y: Number.POSITIVE_INFINITY, z: -1200 };
		const max = { x: 1200, y: Number.NEGATIVE_INFINITY, z: 1200 };

		for (let i = 0; i < textureArray.length; i += 4) {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}

		for (let id = 0; id < nodeCount; id++) {
			const l = level[id] ?? 0;
			const y = -l * ySpacing + y0;
			const base = id * 4;
			textureArray[base] = 0;
			textureArray[base + 1] = y;
			textureArray[base + 2] = 0;
			textureArray[base + 3] = 1.0;

			if (y < min.y) min.y = y;
			if (y > max.y) max.y = y;
		}

		setLayoutBoundsFromMinMax(min, max);
		const texture = new THREE.DataTexture(
			textureArray,
			nodesWidth,
			nodesWidth,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	};

	return {
		generateBfs3dLayoutTexture,
		generateBfsLayoutTexture,
		generateCylinderLayoutTexture,
		generateDag3dLayoutTexture,
		generateDagLayoutTexture,
		generateGuided3dLayoutTexture,
		generateLayeredLayoutTexture,
		generateRadialBfsLayoutTexture,
		generateRadialLayeredLayoutTexture,
		generateSpanningTreeLayoutTexture,
		generateTree3dLayoutTexture,
	};
}

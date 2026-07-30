/**
 * Geometry builders for node graph visualization
 *
 * Creates Three.js buffer geometries for rendering nodes, edges, and labels.
 * All geometries use custom attributes for GPU-based position updates via shaders.
 */

import chroma from "chroma-js";
import * as THREE from "three";
import {
	edgeFragmentShader,
	edgeVertexShader,
	nodeFragmentShader,
	nodeVertexShader,
	pickingFragmentShader,
	textFragmentShader,
	textVertexShader,
} from "../shaders";
import { getTextCoordinates } from "../utils/font-data";
import type { Graph, NodeData } from "./graph";

interface ModuleNodeData extends NodeData {
	kind: "module";
	tensors?: number;
	elements?: number;
	bytes?: number;
}

interface AttentionEdgeData extends NodeData {
	kind: "attn";
	w?: number;
}

const isModuleNodeData = (data: NodeData | undefined): data is ModuleNodeData =>
	data?.kind === "module";

const isAttentionEdgeData = (
	data: NodeData | undefined,
): data is AttentionEdgeData =>
	data?.kind === "attn" && typeof data.w === "number";

export interface LookupTableEntry {
	texPos: [number, number];
	color?: number[];
}

export interface GeometryResult {
	nodeGeometry: THREE.BufferGeometry;
	pickingGeometry: THREE.BufferGeometry;
	edgeGeometry: THREE.BufferGeometry;
	labelGeometry: THREE.BufferGeometry;
	nodeMaterial: THREE.ShaderMaterial;
	pickingMaterial: THREE.ShaderMaterial;
	edgeMaterial: THREE.ShaderMaterial;
	labelMaterial: THREE.ShaderMaterial;
	nodeMesh: THREE.Points;
	pickingMesh: THREE.Points;
	edgeMesh: THREE.LineSegments;
	labelMesh: THREE.Mesh;
	lookupTable: Record<string, LookupTableEntry>;
	nodeNames: string[];
}

// Default color scale for nodes
const DEFAULT_COLOR_SCALE = [
	"#a6cee3",
	"#1f78b4",
	"#b2df8a",
	"#33a02c",
	"#fb9a99",
	"#fdbf6f",
	"#ff7f00",
	"#cab2d6",
	"#6a3d9a",
	"#ffff99",
	"#b15928",
];

function formatBytes(bytes: number): string {
	const b = Math.max(0, Number(bytes) || 0);
	if (b < 1024) return `${Math.round(b)}B`;
	const kb = b / 1024;
	if (kb < 1024) return `${kb.toFixed(kb < 10 ? 1 : 0)}KB`;
	const mb = kb / 1024;
	if (mb < 1024) return `${mb.toFixed(mb < 10 ? 1 : 0)}MB`;
	const gb = mb / 1024;
	return `${gb.toFixed(gb < 10 ? 1 : 0)}GB`;
}

function formatCount(n: number): string {
	const v = Math.max(0, Number(n) || 0);
	if (v < 1_000) return `${Math.round(v)}`;
	if (v < 1_000_000) return `${(v / 1_000).toFixed(v < 10_000 ? 1 : 0)}K`;
	if (v < 1_000_000_000)
		return `${(v / 1_000_000).toFixed(v < 10_000_000 ? 1 : 0)}M`;
	return `${(v / 1_000_000_000).toFixed(v < 10_000_000_000 ? 1 : 0)}B`;
}

function shortenModulePath(key: string): string {
	// Module graph node keys are dotted paths. Keep only the tail so labels remain
	// readable. Must be ASCII for our UbuntuMono atlas.
	const parts = key.split(".");
	if (parts.length <= 3) return key;
	return parts.slice(-3).join(".");
}

export type LabelDetailMode = "compact" | "detailed";

function formatNodeLabel(
	key: string,
	graph: Graph,
	labelDetailMode: LabelDetailMode,
): string {
	const node = graph.nodes[key];
	const data0 = node?.data?.[0];
	if (isModuleNodeData(data0)) {
		if (labelDetailMode === "compact") {
			return shortenModulePath(key);
		}
		const tensors = Number(data0.tensors) || 0;
		const elements = Number(data0.elements) || 0;
		const bytes = Number(data0.bytes) || 0;
		return `${shortenModulePath(key)} ${formatBytes(bytes)} ${tensors}t ${formatCount(elements)}`;
	}

	// Attention visualizer ids are of the form `L{layer}:{tokenIndex}:{token}`.
	// Tokens themselves can be `:` which leads to confusing `...:9::` labels.
	const m = /^L(\d+):(\d+):(.*)$/.exec(key);
	if (!m) return key;

	const layerIndex = m[1] ?? "0";
	const tokenIndex = m[2] ?? "0";
	const token = m[3] ?? "";

	// Keep everything ASCII so it maps to our UbuntuMono atlas.
	const tok = token === "\n" ? "\\n" : token;
	return `L${layerIndex} t${tokenIndex} ${tok}`;
}

/**
 * Create all geometries for the graph
 *
 * Builds node points, edge lines, picking geometry, and text labels.
 * Returns materials and meshes ready to add to a Three.js scene.
 */
export const createGraphGeometry = (
	graph: Graph,
	nodesWidth: number,
	epochsWidth: number,
	nodeTexture: THREE.Texture,
	threatTexture: THREE.Texture,
	fontTexture?: THREE.Texture,
	labelDetailMode: LabelDetailMode = "compact",
): GeometryResult => {
	const nodesCount = graph.getNodeCount();
	const edgesCount = graph.getEdgeCount();

	// Build lookup table mapping node names to texture positions and colors
	const lookupTable: Record<string, LookupTableEntry> = {};
	const nodeNames: string[] = [];
	const chromaScale = chroma.scale(DEFAULT_COLOR_SCALE).domain([0, nodesCount]);

	Object.entries(graph.nodes).forEach(([key, node]) => {
		const nodeIndex = node.id;
		const texStartX = (nodeIndex % nodesWidth) / nodesWidth;
		const texStartY = Math.floor(nodeIndex / nodesWidth) / nodesWidth;
		const color = chromaScale(nodeIndex).gl();

		lookupTable[key] = {
			color: [color[0], color[1], color[2]],
			texPos: [texStartX, texStartY],
		};
		nodeNames[nodeIndex] = key;
	});

	// Create node geometry
	const {
		nodeGeometry,
		pickingGeometry,
		nodeMaterial,
		pickingMaterial,
		nodeMesh,
		pickingMesh,
	} = createNodeGeometry(
		nodesCount,
		nodesWidth,
		epochsWidth,
		lookupTable,
		nodeTexture,
		threatTexture,
		nodeNames,
	);

	// Create edge geometry
	const { edgeGeometry, edgeMaterial, edgeMesh } = createEdgeGeometry(
		graph,
		edgesCount,
		lookupTable,
	);

	// Create label geometry
	const { labelGeometry, labelMaterial, labelMesh } = createLabelGeometry(
		lookupTable,
		fontTexture,
		graph,
		labelDetailMode,
		nodeNames,
	);

	return {
		edgeGeometry,
		edgeMaterial,
		edgeMesh,
		labelGeometry,
		labelMaterial,
		labelMesh,
		lookupTable,
		nodeGeometry,
		nodeMaterial,
		nodeMesh,
		nodeNames,
		pickingGeometry,
		pickingMaterial,
		pickingMesh,
	};
};

const createNodeGeometry = (
	nodesCount: number,
	nodesWidth: number,
	epochsWidth: number,
	lookupTable: Record<string, LookupTableEntry>,
	nodeTexture: THREE.Texture,
	threatTexture: THREE.Texture,
	nodeNames: string[],
): {
	nodeGeometry: THREE.BufferGeometry;
	pickingGeometry: THREE.BufferGeometry;
	nodeMaterial: THREE.ShaderMaterial;
	pickingMaterial: THREE.ShaderMaterial;
	nodeMesh: THREE.Points;
	pickingMesh: THREE.Points;
} => {
	const nodeGeometry = new THREE.BufferGeometry();
	const pickingGeometry = new THREE.BufferGeometry();

	// Visible geometry attributes
	const nodePositions = new Float32Array(nodesCount * 3);
	const nodeReferences = new Float32Array(nodesCount * 2);
	const nodeColors = new Float32Array(nodesCount * 3);
	const nodePick = new Float32Array(nodesCount);
	const threat = new Float32Array(nodesCount);

	// Picking geometry attributes (unique colors for identification)
	const pickingColors = new Float32Array(nodesCount * 3);
	const pickingPick = new Float32Array(nodesCount);

	for (let nodeIndex = 0; nodeIndex < nodesCount; nodeIndex++) {
		const key = nodeNames[nodeIndex];
		const lookup = key ? lookupTable[key] : undefined;

		nodePositions[nodeIndex * 3] = 0;
		nodePositions[nodeIndex * 3 + 1] = 0;
		nodePositions[nodeIndex * 3 + 2] = 0;

		if (lookup?.color) {
			nodeColors[nodeIndex * 3] = lookup.color[0];
			nodeColors[nodeIndex * 3 + 1] = lookup.color[1];
			nodeColors[nodeIndex * 3 + 2] = lookup.color[2];
		}

		const encodedId = nodeIndex + 1;
		pickingColors[nodeIndex * 3] = ((encodedId >> 16) & 255) / 255;
		pickingColors[nodeIndex * 3 + 1] = ((encodedId >> 8) & 255) / 255;
		pickingColors[nodeIndex * 3 + 2] = (encodedId & 255) / 255;

		nodePick[nodeIndex] = 1.0;
		pickingPick[nodeIndex] = 0.0;

		if (lookup) {
			nodeReferences[nodeIndex * 2] = lookup.texPos[0];
			nodeReferences[nodeIndex * 2 + 1] = lookup.texPos[1];
		}

		threat[nodeIndex] = 0;
	}

	// Set up visible geometry
	nodeGeometry.setAttribute(
		"position",
		new THREE.BufferAttribute(nodePositions, 3),
	);
	nodeGeometry.setAttribute(
		"texPos",
		new THREE.BufferAttribute(nodeReferences, 2),
	);
	nodeGeometry.setAttribute(
		"customColor",
		new THREE.BufferAttribute(nodeColors, 3),
	);
	nodeGeometry.setAttribute(
		"pickingNode",
		new THREE.BufferAttribute(nodePick, 1),
	);
	nodeGeometry.setAttribute("threat", new THREE.BufferAttribute(threat, 1));

	// Set up picking geometry (shares position and texPos)
	pickingGeometry.setAttribute(
		"position",
		new THREE.BufferAttribute(nodePositions, 3),
	);
	pickingGeometry.setAttribute(
		"texPos",
		new THREE.BufferAttribute(nodeReferences, 2),
	);
	pickingGeometry.setAttribute(
		"customColor",
		new THREE.BufferAttribute(pickingColors, 3),
	);
	pickingGeometry.setAttribute(
		"pickingNode",
		new THREE.BufferAttribute(pickingPick, 1),
	);
	pickingGeometry.setAttribute("threat", new THREE.BufferAttribute(threat, 1));

	// Node uniforms
	const nodeUniforms = {
		currentTime: { value: 0.0 },
		nodeAttribTexture: { value: null as THREE.Texture | null },
		nodeIntensityTexture: { value: null as THREE.Texture | null },
		positionTexture: { value: null as THREE.Texture | null },
		sprite: { value: nodeTexture },
		threatSprite: { value: threatTexture },
	};

	// Visible node material
	const nodeMaterial = new THREE.ShaderMaterial({
		blending: THREE.AdditiveBlending,
		defines: {
			EPOCHSWIDTH: epochsWidth.toFixed(2),
		},
		depthTest: false,
		fragmentShader: nodeFragmentShader,
		transparent: true,
		uniforms: nodeUniforms,
		vertexShader: nodeVertexShader,
	});

	// Picking material needs its own uniforms (no blending for accurate color reading)
	const pickingUniforms = {
		currentTime: { value: 0.0 },
		nodeAttribTexture: { value: null as THREE.Texture | null },
		nodeIntensityTexture: { value: null as THREE.Texture | null },
		positionTexture: { value: null as THREE.Texture | null },
		sprite: { value: nodeTexture },
		threatSprite: { value: threatTexture },
	};

	const pickingMaterial = new THREE.ShaderMaterial({
		defines: {
			EPOCHSWIDTH: epochsWidth.toFixed(2),
			NODESWIDTH: nodesWidth.toFixed(2),
		},
		depthTest: false,
		fragmentShader: pickingFragmentShader,
		transparent: false,
		uniforms: pickingUniforms,
		vertexShader: nodeVertexShader,
	});

	const nodeMesh = new THREE.Points(nodeGeometry, nodeMaterial);
	const pickingMesh = new THREE.Points(pickingGeometry, pickingMaterial);

	return {
		nodeGeometry,
		nodeMaterial,
		nodeMesh,
		pickingGeometry,
		pickingMaterial,
		pickingMesh,
	};
};

function createEdgeGeometry(
	graph: Graph,
	edgesCount: number,
	lookupTable: Record<string, LookupTableEntry>,
) {
	const edgeGeometry = new THREE.BufferGeometry();

	// Each edge has 2 vertices (start and end)
	const edgePositions = new Float32Array(edgesCount * 2 * 3);
	const edgeReferences = new Float32Array(edgesCount * 2 * 2);
	const edgeColors = new Float32Array(edgesCount * 2 * 3);

	let v = 0;
	Object.entries(graph.edges).forEach(([_key, edge]) => {
		const startNode = lookupTable[edge.source];
		const endNode = lookupTable[edge.target];

		if (!startNode || !endNode) return;

		// Optional: attention edges can include a weight `w` in their data.
		// We use this to modulate edge brightness for readability (cheap + effective).
		// Minimum factor increased from 0.2 to 0.4 for better visibility.
		let weightFactor = 1.0;
		for (const edgeDatum of edge.data ?? []) {
			if (isAttentionEdgeData(edgeDatum)) {
				const weight = Math.max(0, Math.min(1, edgeDatum.w));
				weightFactor = 0.4 + 0.6 * Math.sqrt(weight);
				break;
			}
		}

		// Start of line
		edgeReferences[v * 2] = startNode.texPos[0];
		edgeReferences[v * 2 + 1] = startNode.texPos[1];

		edgePositions[v * 3] = 0;
		edgePositions[v * 3 + 1] = 0;
		edgePositions[v * 3 + 2] = 0;

		if (startNode.color) {
			edgeColors[v * 3] = startNode.color[0] * weightFactor;
			edgeColors[v * 3 + 1] = startNode.color[1] * weightFactor;
			edgeColors[v * 3 + 2] = startNode.color[2] * weightFactor;
		}

		v++;

		// End of line
		edgeReferences[v * 2] = endNode.texPos[0];
		edgeReferences[v * 2 + 1] = endNode.texPos[1];

		edgePositions[v * 3] = 0;
		edgePositions[v * 3 + 1] = 0;
		edgePositions[v * 3 + 2] = 0;

		if (endNode.color) {
			edgeColors[v * 3] = endNode.color[0] * weightFactor;
			edgeColors[v * 3 + 1] = endNode.color[1] * weightFactor;
			edgeColors[v * 3 + 2] = endNode.color[2] * weightFactor;
		}

		v++;
	});

	edgeGeometry.setAttribute(
		"position",
		new THREE.BufferAttribute(edgePositions, 3),
	);
	edgeGeometry.setAttribute(
		"texPos",
		new THREE.BufferAttribute(edgeReferences, 2),
	);
	edgeGeometry.setAttribute(
		"customColor",
		new THREE.BufferAttribute(edgeColors, 3),
	);

	const edgeUniforms = {
		nodeAttribTexture: { value: null as THREE.Texture | null },
		nodeIntensityTexture: { value: null as THREE.Texture | null },
		positionTexture: { value: null as THREE.Texture | null },
	};

	const edgeMaterial = new THREE.ShaderMaterial({
		depthTest: false,
		fragmentShader: edgeFragmentShader,
		transparent: true,
		uniforms: edgeUniforms,
		vertexShader: edgeVertexShader,
	});

	const edgeMesh = new THREE.LineSegments(edgeGeometry, edgeMaterial);

	return { edgeGeometry, edgeMaterial, edgeMesh };
}

function createLabelGeometry(
	lookupTable: Record<string, LookupTableEntry>,
	fontTexture: THREE.Texture | undefined,
	graph: Graph,
	labelDetailMode: LabelDetailMode,
	nodeNames: string[],
) {
	// Count total characters needed
	let particleCount = 0;
	nodeNames.forEach((key) => {
		const label = formatNodeLabel(key, graph, labelDetailMode);
		particleCount += label.length;
	});

	const labelGeometry = new THREE.BufferGeometry();

	// 6 vertices per character (2 triangles forming a quad)
	const positions = new Float32Array(particleCount * 6 * 3);
	const labelPositions = new Float32Array(particleCount * 6 * 3);
	const labelColors = new Float32Array(particleCount * 6 * 3);
	const uvs = new Float32Array(particleCount * 6 * 2);
	const ids = new Float32Array(particleCount * 6);
	const textCoords = new Float32Array(particleCount * 6 * 4);
	const labelReferences = new Float32Array(particleCount * 6 * 2);

	// Character size settings - matches original
	const letterWidth = 20;
	const letterSpacing = 15;

	let counter = 0;
	nodeNames.forEach((key) => {
		const nodeLookup = lookupTable[key];
		if (!nodeLookup) return;

		const label = formatNodeLabel(key, graph, labelDetailMode);
		for (let i = 0; i < label.length; i++) {
			const index = counter * 3 * 2; // Same indexing as original

			// Get texture coordinates for this character using proper font atlas data
			const tc = getTextCoordinates(label[i]);
			// tc = [left, top, width, height, xoffset, yoffset] - all normalized to 0-1

			// Compute vertex positions using character metrics
			// Left is offset
			const l = tc[4];
			// Right is offset + width
			const r = tc[4] + tc[2];
			// Bottom is y offset - height
			const b = tc[5] - tc[3];
			// Top is y offset
			const t = tc[5];

			// Set character IDs
			ids[index + 0] = i;
			ids[index + 1] = i;
			ids[index + 2] = i;
			ids[index + 3] = i;
			ids[index + 4] = i;
			ids[index + 5] = i;

			// Set positions (will be updated by shader - all zeros)
			for (let j = 0; j < 18; j++) {
				positions[index * 3 + j] = 0;
			}

			// Label positions - vertex offsets from node center
			// Uses font metrics for proper character spacing and alignment
			// Triangle 1: top-left, bottom-left, top-right
			labelPositions[index * 3 + 0] = i * letterSpacing + l * letterWidth * 10;
			labelPositions[index * 3 + 1] = t * letterWidth * 10;
			labelPositions[index * 3 + 2] = 0 * letterWidth * 10;

			labelPositions[index * 3 + 3] = i * letterSpacing + l * letterWidth * 10;
			labelPositions[index * 3 + 4] = b * letterWidth * 10;
			labelPositions[index * 3 + 5] = 0 * letterWidth * 10;

			labelPositions[index * 3 + 6] = i * letterSpacing + r * letterWidth * 10;
			labelPositions[index * 3 + 7] = t * letterWidth * 10;
			labelPositions[index * 3 + 8] = 0 * letterWidth * 10;

			// Triangle 2: bottom-right, top-right, bottom-left
			labelPositions[index * 3 + 9] = i * letterSpacing + r * letterWidth * 10;
			labelPositions[index * 3 + 10] = b * letterWidth * 10;
			labelPositions[index * 3 + 11] = 0 * letterWidth * 10;

			labelPositions[index * 3 + 12] = i * letterSpacing + r * letterWidth * 10;
			labelPositions[index * 3 + 13] = t * letterWidth * 10;
			labelPositions[index * 3 + 14] = 0 * letterWidth * 10;

			labelPositions[index * 3 + 15] = i * letterSpacing + l * letterWidth * 10;
			labelPositions[index * 3 + 16] = b * letterWidth * 10;
			labelPositions[index * 3 + 17] = 0 * letterWidth * 10;

			// UV coordinates for the quad - same for all
			uvs[index * 2 + 0] = 0;
			uvs[index * 2 + 1] = 1;

			uvs[index * 2 + 2] = 0;
			uvs[index * 2 + 3] = 0;

			uvs[index * 2 + 4] = 1;
			uvs[index * 2 + 5] = 1;

			uvs[index * 2 + 6] = 1;
			uvs[index * 2 + 7] = 0;

			uvs[index * 2 + 8] = 1;
			uvs[index * 2 + 9] = 1;

			uvs[index * 2 + 10] = 0;
			uvs[index * 2 + 11] = 0;

			// Text coordinates - same for all 6 vertices of this character
			// tc[0] = left, tc[1] = top, tc[2] = width, tc[3] = height (all normalized)
			textCoords[index * 4 + 0] = tc[0];
			textCoords[index * 4 + 1] = tc[1];
			textCoords[index * 4 + 2] = tc[2];
			textCoords[index * 4 + 3] = tc[3];

			textCoords[index * 4 + 4] = tc[0];
			textCoords[index * 4 + 5] = tc[1];
			textCoords[index * 4 + 6] = tc[2];
			textCoords[index * 4 + 7] = tc[3];

			textCoords[index * 4 + 8] = tc[0];
			textCoords[index * 4 + 9] = tc[1];
			textCoords[index * 4 + 10] = tc[2];
			textCoords[index * 4 + 11] = tc[3];

			textCoords[index * 4 + 12] = tc[0];
			textCoords[index * 4 + 13] = tc[1];
			textCoords[index * 4 + 14] = tc[2];
			textCoords[index * 4 + 15] = tc[3];

			textCoords[index * 4 + 16] = tc[0];
			textCoords[index * 4 + 17] = tc[1];
			textCoords[index * 4 + 18] = tc[2];
			textCoords[index * 4 + 19] = tc[3];

			textCoords[index * 4 + 20] = tc[0];
			textCoords[index * 4 + 21] = tc[1];
			textCoords[index * 4 + 22] = tc[2];
			textCoords[index * 4 + 23] = tc[3];

			// Label references to node position texture - same for all 6 vertices
			labelReferences[index * 2 + 0] = nodeLookup.texPos[0];
			labelReferences[index * 2 + 1] = nodeLookup.texPos[1];

			labelReferences[index * 2 + 2] = nodeLookup.texPos[0];
			labelReferences[index * 2 + 3] = nodeLookup.texPos[1];

			labelReferences[index * 2 + 4] = nodeLookup.texPos[0];
			labelReferences[index * 2 + 5] = nodeLookup.texPos[1];

			labelReferences[index * 2 + 6] = nodeLookup.texPos[0];
			labelReferences[index * 2 + 7] = nodeLookup.texPos[1];

			labelReferences[index * 2 + 8] = nodeLookup.texPos[0];
			labelReferences[index * 2 + 9] = nodeLookup.texPos[1];

			labelReferences[index * 2 + 10] = nodeLookup.texPos[0];
			labelReferences[index * 2 + 11] = nodeLookup.texPos[1];

			// Label colors from node - same for all 6 vertices
			labelColors[index * 3 + 0] = nodeLookup.color?.[0] ?? 1;
			labelColors[index * 3 + 1] = nodeLookup.color?.[1] ?? 1;
			labelColors[index * 3 + 2] = nodeLookup.color?.[2] ?? 1;

			labelColors[index * 3 + 3] = nodeLookup.color?.[0] ?? 1;
			labelColors[index * 3 + 4] = nodeLookup.color?.[1] ?? 1;
			labelColors[index * 3 + 5] = nodeLookup.color?.[2] ?? 1;

			labelColors[index * 3 + 6] = nodeLookup.color?.[0] ?? 1;
			labelColors[index * 3 + 7] = nodeLookup.color?.[1] ?? 1;
			labelColors[index * 3 + 8] = nodeLookup.color?.[2] ?? 1;

			labelColors[index * 3 + 9] = nodeLookup.color?.[0] ?? 1;
			labelColors[index * 3 + 10] = nodeLookup.color?.[1] ?? 1;
			labelColors[index * 3 + 11] = nodeLookup.color?.[2] ?? 1;

			labelColors[index * 3 + 12] = nodeLookup.color?.[0] ?? 1;
			labelColors[index * 3 + 13] = nodeLookup.color?.[1] ?? 1;
			labelColors[index * 3 + 14] = nodeLookup.color?.[2] ?? 1;

			labelColors[index * 3 + 15] = nodeLookup.color?.[0] ?? 1;
			labelColors[index * 3 + 16] = nodeLookup.color?.[1] ?? 1;
			labelColors[index * 3 + 17] = nodeLookup.color?.[2] ?? 1;

			counter++;
		}
	});

	labelGeometry.setAttribute(
		"position",
		new THREE.BufferAttribute(positions, 3),
	);
	labelGeometry.setAttribute(
		"labelPositions",
		new THREE.BufferAttribute(labelPositions, 3),
	);
	labelGeometry.setAttribute("uv", new THREE.BufferAttribute(uvs, 2));
	labelGeometry.setAttribute("id", new THREE.BufferAttribute(ids, 1));
	labelGeometry.setAttribute(
		"textCoord",
		new THREE.BufferAttribute(textCoords, 4),
	);
	labelGeometry.setAttribute(
		"texPos",
		new THREE.BufferAttribute(labelReferences, 2),
	);
	labelGeometry.setAttribute(
		"customColor",
		new THREE.BufferAttribute(labelColors, 3),
	);

	const labelUniforms = {
		nodeAttribTexture: { value: null as THREE.Texture | null },
		nodeIntensityTexture: { value: null as THREE.Texture | null },
		positionTexture: { value: null as THREE.Texture | null },
		t_text: { value: fontTexture ?? null },
	};

	const labelMaterial = new THREE.ShaderMaterial({
		depthTest: false,
		fragmentShader: textFragmentShader,
		transparent: true,
		uniforms: labelUniforms,
		vertexShader: textVertexShader,
	});

	const labelMesh = new THREE.Mesh(labelGeometry, labelMaterial);

	return { labelGeometry, labelMaterial, labelMesh };
}

/**
 * Update texture uniforms for all materials
 *
 * Called each frame to update position and attribute textures
 * from the GPU simulator.
 */
export function updateMaterialTextures(
	result: GeometryResult,
	positionTexture: THREE.Texture | null,
	nodeAttribTexture: THREE.Texture | null,
	nodeIntensityTexture?: THREE.Texture | null,
): void {
	// Update node materials
	result.nodeMaterial.uniforms.positionTexture.value = positionTexture;
	result.nodeMaterial.uniforms.nodeAttribTexture.value = nodeAttribTexture;
	if (nodeIntensityTexture !== undefined) {
		result.nodeMaterial.uniforms.nodeIntensityTexture.value =
			nodeIntensityTexture;
		result.pickingMaterial.uniforms.nodeIntensityTexture.value =
			nodeIntensityTexture;
		result.edgeMaterial.uniforms.nodeIntensityTexture.value =
			nodeIntensityTexture;
		result.labelMaterial.uniforms.nodeIntensityTexture.value =
			nodeIntensityTexture;
	}
	result.pickingMaterial.uniforms.positionTexture.value = positionTexture;
	result.pickingMaterial.uniforms.nodeAttribTexture.value = nodeAttribTexture;

	// Update edge material
	result.edgeMaterial.uniforms.positionTexture.value = positionTexture;
	result.edgeMaterial.uniforms.nodeAttribTexture.value = nodeAttribTexture;

	// Update label material
	result.labelMaterial.uniforms.positionTexture.value = positionTexture;
	result.labelMaterial.uniforms.nodeAttribTexture.value = nodeAttribTexture;
}

/**
 * Dispose all geometries and materials
 */
export function disposeGeometry(result: GeometryResult): void {
	result.nodeGeometry.dispose();
	result.pickingGeometry.dispose();
	result.edgeGeometry.dispose();
	result.labelGeometry.dispose();
	result.nodeMaterial.dispose();
	result.pickingMaterial.dispose();
	result.edgeMaterial.dispose();
	result.labelMaterial.dispose();
}

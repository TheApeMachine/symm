import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { BokehPass } from "three/examples/jsm/postprocessing/BokehPass.js";
import { EffectComposer } from "three/examples/jsm/postprocessing/EffectComposer.js";
import { RenderPass } from "three/examples/jsm/postprocessing/RenderPass.js";
import {
	createGraphGeometry,
	disposeGeometry,
	type GeometryResult,
	updateMaterialTextures,
} from "./core/geometry";
import { GPUPicking, type MouseState, MouseTracker } from "./core/gpu-picking";
import { Graph, generators } from "./core/graph";
import { Simulator, type SimulatorConfig } from "./core/simulator";
import type { TimeSliderHandle } from "./timeslider.tsx";
import {
	dataTextureSize,
	generateCircularLayout,
	generateGridLayout,
	generateHelixLayout,
	generateIntensityTexture,
	generateSphericalLayout,
	generateZeroedPositionTexture,
	indexTextureSize,
	type NodeMetrics,
} from "./utils/texture-generators";
import "./styles/slider-styles.css";
import { Camera } from "./core/camera";
import {
	createLayoutTextureGenerators,
	type LayoutType,
	normalizeLayout,
} from "./layout-textures";
import { NodeGraphLegacyLayoutControls } from "./layoutcontrols.tsx";
import { NodeGraphLegacyTimeControls } from "./timecontrols.tsx";
import { loadNodeTexture, loadThreatTexture } from "./utils/texture-loaders";

const LAYOUT_STORAGE_KEY = "caramba.nodeGraphLegacy.layout";
const CAMERA_AUTOFIT_STORAGE_KEY = "caramba.nodeGraphLegacy.camera.autoFit";

const loadFontTexture = async (): Promise<THREE.Texture> => {
	return new Promise((resolve) => {
		const loader = new THREE.TextureLoader();
		loader.load(
			"/fonts/UbuntuMono.png",
			(texture: THREE.Texture) => {
				texture.flipY = false;
				texture.magFilter = THREE.LinearFilter;
				texture.minFilter = THREE.LinearFilter;
				texture.needsUpdate = true;
				console.log("[Font] Loaded UbuntuMono font texture");
				resolve(texture);
			},
			undefined,
			(error: unknown) => {
				console.warn(
					"[Font] Failed to load font texture, labels disabled:",
					error,
				);
				resolve(new THREE.Texture());
			},
		);
	});
};

export type TimeRange = {
	min: number;
	max: number;
	from: number;
	to: number;
};

/**
 * Extract normalized metrics from graph node data
 *
 * Reads size_norm, brightness_norm, weight_mag_norm from node data
 * (set by the Python backend) and returns an ordered array matching
 * the node order in the graph.
 */
function extractNodeMetrics(graph: Graph): NodeMetrics[] {
	const nodeNames = Object.keys(graph.nodes);
	const metrics: NodeMetrics[] = [];

	for (const name of nodeNames) {
		const node = graph.nodes[name];
		const data = node?.data?.[0] as Record<string, unknown> | undefined;

		// Extract normalized values from backend data (default to 0.5 if missing)
		const sizeNorm = typeof data?.size_norm === "number" ? data.size_norm : 0.5;
		const brightnessNorm =
			typeof data?.brightness_norm === "number" ? data.brightness_norm : 0.5;
		const weightMagNorm =
			typeof data?.weight_mag_norm === "number" ? data.weight_mag_norm : 0.5;

		metrics.push({
			sizeNorm,
			brightnessNorm,
			weightMagNorm,
		});
	}

	return metrics;
}

export interface ModelScopeProps {
	graph?: Graph;
	layout?: LayoutType;
	initialTemperature?: number;
	coolingRate?: number;
	epochMin?: number;
	epochMax?: number;
	showLabels?: boolean;
	labelDetailMode?: "compact" | "detailed";
	showEdges?: boolean;
	showTimeSlider?: boolean;
	nodeIntensity?: number[];
	nodeThreat?: number[];
	onNodeSelect?: (nodeIndex: number, nodeName: string) => void;
	onNodeHover?: (nodeIndex: number, nodeName: string) => void;
	/**
	 * Controlled selection. When provided, drives the simulator's
	 * highlighted node so external panels can select nodes too.
	 */
	selectedNodeName?: string | null;
	onTimeRangeChange?: (from: number, to: number) => void;
	className?: string;
	/**
	 * Contrast multiplier for size/brightness differences between nodes.
	 * - 1.0 = subtle (original normalized values)
	 * - 2.0 = moderate (recommended, default)
	 * - 3.0 = strong (very noticeable differences)
	 */
	metricsContrast?: number;
}

export function ModelScope({
	graph: externalGraph,
	layout = "force",
	initialTemperature = 500,
	coolingRate = 0.99,
	epochMin: externalEpochMin,
	epochMax: externalEpochMax,
	showLabels = true,
	labelDetailMode = "compact",
	showEdges = true,
	showTimeSlider = true,
	nodeIntensity,
	nodeThreat,
	onNodeSelect,
	onNodeHover,
	onTimeRangeChange,
	className,
	metricsContrast = 2.0,
	selectedNodeName,
}: ModelScopeProps) {
	const containerRef = useRef<HTMLDivElement>(null);
	const rendererRef = useRef<THREE.WebGLRenderer | null>(null);
	const composerRef = useRef<EffectComposer | null>(null);
	const bokehPassRef = useRef<BokehPass | null>(null);
	const sceneRef = useRef<THREE.Scene | null>(null);
	const cameraRef = useRef<THREE.PerspectiveCamera | null>(null);
	const controlsRef = useRef<OrbitControls | null>(null);
	const simulatorRef = useRef<Simulator | null>(null);
	const pickingRef = useRef<GPUPicking | null>(null);
	const mouseTrackerRef = useRef<MouseTracker | null>(null);
	const geometryResultRef = useRef<GeometryResult | null>(null);
	const graphStructureRef = useRef<THREE.Group | null>(null);
	const pickingSceneRef = useRef<THREE.Scene | null>(null);
	const nodeIntensityTextureRef = useRef<THREE.DataTexture | null>(null);
	const nodeIntensityRef = useRef(nodeIntensity);
	nodeIntensityRef.current = nodeIntensity;
	const nodeThreatRef = useRef(nodeThreat);
	nodeThreatRef.current = nodeThreat;

	const sliderRef = useRef<TimeSliderHandle>(null);

	const [isPlaying, setIsPlaying] = useState(false);
	const [isRepeating, setIsRepeating] = useState(true);
	const [isExpanded, setIsExpanded] = useState(false);
	const [isDofEnabled, setIsDofEnabled] = useState(false);
	const [autoFitCamera, setAutoFitCamera] = useState<boolean>(() => {
		if (typeof window === "undefined") return false;
		const saved = window.localStorage.getItem(CAMERA_AUTOFIT_STORAGE_KEY);
		return saved === "1";
	});

	const animationFrameRef = useRef<number>(0);
	const lastMouseStateRef = useRef<MouseState>({
		dblClick: false,
		down: false,
		up: false,
		x: 0,
		y: 0,
	});
	const temperatureRef = useRef<number>(initialTemperature);
	const [currentLayout, setCurrentLayout] = useState<LayoutType>(() => {
		const fallback = normalizeLayout(layout, "force");
		if (typeof window === "undefined") return fallback;
		const saved = window.localStorage.getItem(LAYOUT_STORAGE_KEY);
		return normalizeLayout(saved, fallback);
	});
	const [isSimulating, setIsSimulating] = useState(true);
	const [graphVersion, setGraphVersion] = useState(0);
	const graphRef = useRef<Graph | null>(null);

	const selectedNodeRef = useRef<string | null>(null);
	const hoveredNodeRef = useRef<string | null>(null);

	const [timeRange, setTimeRange] = useState<TimeRange>({
		from: 0,
		max: 1000,
		min: 0,
		to: 100,
	});

	const epochMin = externalEpochMin ?? timeRange.from;
	const epochMax = externalEpochMax ?? timeRange.to;

	const configRef = useRef<{
		nodesWidth: number;
		edgesWidth: number;
		epochsWidth: number;
		epochOffset: number;
	} | null>(null);
	const lastLayoutBoundsRef = useRef<{
		center: THREE.Vector3;
		radius: number;
	} | null>(null);
	const lastCameraSaveMsRef = useRef<number>(0);
	const backgroundTextureRef = useRef<THREE.Texture | null>(null);

	const {
		fitCameraToGraph,
		restoreCameraState,
		saveCameraState,
		setOrbitTargetToLayoutCenter,
	} = useMemo(
		() =>
			Camera({
				cameraRef,
				controlsRef,
				lastCameraSaveMsRef,
				lastLayoutBoundsRef,
				simulatorRef,
			}),
		[],
	);

	const setLayoutBoundsFromMinMax = useCallback(
		(
			min: { x: number; y: number; z: number },
			max: { x: number; y: number; z: number },
		) => {
			const center = new THREE.Vector3(
				(min.x + max.x) / 2,
				(min.y + max.y) / 2,
				(min.z + max.z) / 2,
			);
			const dx = max.x - min.x;
			const dy = max.y - min.y;
			const dz = max.z - min.z;
			const radius = Math.max(50, 0.5 * Math.sqrt(dx * dx + dy * dy + dz * dz));
			lastLayoutBoundsRef.current = { center, radius };
		},
		[],
	);

	const {
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
	} = useMemo(
		() =>
			createLayoutTextureGenerators({
				getGraph: () => graphRef.current,
				setLayoutBoundsFromMinMax,
			}),
		[setLayoutBoundsFromMinMax],
	);

	const applyThreatMask = useCallback((threatMask: number[] | undefined) => {
		if (!geometryResultRef.current) return;

		const nodeAttr = geometryResultRef.current.nodeGeometry.getAttribute(
			"threat",
		) as THREE.BufferAttribute | null;
		const pickingAttr = geometryResultRef.current.pickingGeometry.getAttribute(
			"threat",
		) as THREE.BufferAttribute | null;

		if (!nodeAttr || !pickingAttr) return;
		if (!(nodeAttr.array instanceof Float32Array)) return;

		const arr = nodeAttr.array;
		arr.fill(0);
		if (threatMask && threatMask.length > 0) {
			const n = Math.min(arr.length, threatMask.length);
			for (let i = 0; i < n; i++) arr[i] = threatMask[i] ? 1 : 0;
		}
		nodeAttr.needsUpdate = true;
		pickingAttr.needsUpdate = true;
	}, []);

	const initScene = useCallback(() => {
		if (!containerRef.current) return;

		const container = containerRef.current;
		const width = container.clientWidth || window.innerWidth;
		const height = container.clientHeight || window.innerHeight;
		const dpr = window.devicePixelRatio;

		const renderer = new THREE.WebGLRenderer({
			alpha: true,
			antialias: true,
		});
		// Match original: manual DPR scaling with false flag to not update CSS
		renderer.setSize(width * dpr, height * dpr, false);
		renderer.setClearColor(0x000000, 0);
		// Set canvas CSS size explicitly
		renderer.domElement.style.width = `${width}px`;
		renderer.domElement.style.height = `${height}px`;
		// Keep renderer background transparent; gradient lives on the container.
		renderer.domElement.style.background = "transparent";
		container.appendChild(renderer.domElement);
		rendererRef.current = renderer;

		const scene = new THREE.Scene();
		sceneRef.current = scene;

		const pickingScene = new THREE.Scene();
		pickingSceneRef.current = pickingScene;

		const camera = new THREE.PerspectiveCamera(50, width / height, 0.1, 50000);
		camera.position.z = 1500;
		cameraRef.current = camera;

		scene.background = null;

		// Postprocessing composer for optional depth-of-field.
		const composer = new EffectComposer(renderer);
		composer.setSize(width * dpr, height * dpr);
		composerRef.current = composer;

		const renderPass = new RenderPass(scene, camera);
		composer.addPass(renderPass);

		// BokehPass expects focus distance in world units.
		const bokehParams: ConstructorParameters<typeof BokehPass>[2] & {
			width: number;
			height: number;
		} = {
			aperture: 0.00002, // significantly reduced to widen the in-focus area
			focus: 0.0,
			height: height * dpr,
			maxblur: 0.001, // reduced max blur to avoid obliterating the background
			width: width * dpr,
		};
		const bokehPass = new BokehPass(scene, camera, bokehParams);
		bokehPass.enabled = false;
		composer.addPass(bokehPass);
		bokehPassRef.current = bokehPass;

		const controls = new OrbitControls(camera, renderer.domElement);
		// Legacy controls settings
		controls.dampingFactor = 0.2;
		controls.enableDamping = false; // Legacy had enableDamping=false but damping=0.2 set. Typically requires update() loop if true.
		// Let's match legacy: controls.enableDamping = false
		controls.rotateSpeed = 1.0; // Default
		controls.zoomSpeed = 1.0;
		controls.minDistance = 1;
		controls.maxDistance = 500000;
		controls.enablePan = true;
		controls.panSpeed = 0.8;
		controls.screenSpacePanning = true;
		controlsRef.current = controls;
		restoreCameraState();
		controls.addEventListener("change", saveCameraState);

		const graphStructure = new THREE.Group();
		scene.add(graphStructure);
		graphStructureRef.current = graphStructure;

		const simulator = new Simulator(renderer);
		simulatorRef.current = simulator;

		const mouseTracker = new MouseTracker(renderer.domElement);
		mouseTrackerRef.current = mouseTracker;

		return () => {
			backgroundTextureRef.current?.dispose();
			backgroundTextureRef.current = null;

			renderer.dispose();
			container.removeChild(renderer.domElement);
			mouseTracker.dispose();
			controls.removeEventListener("change", saveCameraState);
			controls.dispose();
			bokehPassRef.current = null;
			composerRef.current?.dispose();
			composerRef.current = null;
		};
	}, [restoreCameraState, saveCameraState]);

	const loadGraph = useCallback(
		async (graph: Graph) => {
			graphRef.current = graph;
			if (
				!rendererRef.current ||
				!simulatorRef.current ||
				!graphStructureRef.current ||
				!pickingSceneRef.current
			) {
				return;
			}

			if (geometryResultRef.current) {
				disposeGeometry(geometryResultRef.current);
				graphStructureRef.current.clear();
				pickingSceneRef.current.clear();
			}

			const nodesCount = graph.getNodeCount();
			if (nodesCount === 0) return;

			const nodesWidth = indexTextureSize(graph.getNodesAndEdgesArray());
			const edgesWidth = dataTextureSize(graph.getNodesAndEdgesArray());
			const nodesAndEpochs = graph.getEpochTextureArray("nodes");
			const epochsWidth = dataTextureSize(nodesAndEpochs);

			let epochOffset = Number.MAX_SAFE_INTEGER;
			let epochMaxValue = 0;
			nodesAndEpochs.forEach((epochs) => {
				if (epochs) {
					epochs.forEach((epoch) => {
						if (epoch < epochOffset) epochOffset = epoch;
						if (epoch > epochMaxValue) epochMaxValue = epoch;
					});
				}
			});
			if (epochOffset === Number.MAX_SAFE_INTEGER) epochOffset = 0;
			if (epochMaxValue === 0) epochMaxValue = 1000;

			const timeSpan = epochMaxValue - epochOffset;
			const initialWindow = timeSpan / 25;
			setTimeRange({
				from: 0,
				max: timeSpan,
				min: 0,
				to: initialWindow,
			});

			configRef.current = {
				edgesWidth,
				epochOffset,
				epochsWidth,
				nodesWidth,
			};

			if (nodeIntensityTextureRef.current) {
				nodeIntensityTextureRef.current.dispose();
			}
			nodeIntensityTextureRef.current = generateIntensityTexture(
				nodeIntensityRef.current,
				nodesWidth,
			);

			// Extract normalized metrics from graph for size/brightness variation
			const nodeMetrics = extractNodeMetrics(graph);

			const simulatorConfig: SimulatorConfig = {
				edgesWidth,
				epochOffset,
				epochsWidth,
				nodesAndEdges: graph.getNodesAndEdgesArray(),
				nodesAndEpochs,
				nodesWidth,
				nodeMetrics,
				nodeAttribConfig: { contrast: metricsContrast },
			};
			simulatorRef.current.init(simulatorConfig);

			const nodeTexture = await loadNodeTexture();
			const threatTexture = await loadThreatTexture();
			const fontTexture = await loadFontTexture();

			const geometryResult = createGraphGeometry(
				graph,
				nodesWidth,
				epochsWidth,
				nodeTexture,
				threatTexture,
				fontTexture,
				labelDetailMode,
			);
			geometryResultRef.current = geometryResult;

			graphStructureRef.current.add(geometryResult.nodeMesh);
			graphStructureRef.current.add(geometryResult.edgeMesh);
			graphStructureRef.current.add(geometryResult.labelMesh);
			geometryResult.labelMesh.visible = showLabels;
			pickingSceneRef.current.add(geometryResult.pickingMesh);

			applyThreatMask(nodeThreatRef.current);

			if (cameraRef.current) {
				const picking = new GPUPicking(
					rendererRef.current,
					pickingSceneRef.current,
					cameraRef.current,
					simulatorRef.current,
				);
				picking.setNodeNames(geometryResult.nodeNames);
				const pickWidth =
					containerRef.current?.clientWidth || window.innerWidth;
				const pickHeight =
					containerRef.current?.clientHeight || window.innerHeight;
				picking.resize(pickWidth, pickHeight);

				picking.onNodeSelect = (index, name) => {
					selectedNodeRef.current = name;
					onNodeSelect?.(index, name);
				};

				picking.onNodeHover = (index, name) => {
					hoveredNodeRef.current = name;
					onNodeHover?.(index, name);
				};

				picking.onSelectionClear = () => {
					selectedNodeRef.current = null;
					hoveredNodeRef.current = null;
				};

				pickingRef.current = picking;
			}

			const nodeCount = graph.getNodeCount();
			temperatureRef.current = nodeCount / 2;
			setIsSimulating(true);

			if (autoFitCamera) setTimeout(() => fitCameraToGraph(), 100);
			setGraphVersion((v) => v + 1);
		},
		[
			showLabels,
			labelDetailMode,
			onNodeSelect,
			onNodeHover,
			fitCameraToGraph,
			applyThreatMask,
			autoFitCamera,
			metricsContrast,
		],
	);

	const animate = useCallback(() => {
		animationFrameRef.current = requestAnimationFrame(animate);

		if (
			!rendererRef.current ||
			!sceneRef.current ||
			!cameraRef.current ||
			!simulatorRef.current
		) {
			return;
		}

		// Match analytics-master: drive threat "pulse" animation via currentTime uniform.
		// The legacy shaders expect a monotonically increasing millisecond timer (performance.now()).
		if (geometryResultRef.current) {
			const now = performance.now();
			if (geometryResultRef.current.nodeMaterial.uniforms.currentTime) {
				geometryResultRef.current.nodeMaterial.uniforms.currentTime.value = now;
			}
			if (geometryResultRef.current.pickingMaterial.uniforms.currentTime) {
				geometryResultRef.current.pickingMaterial.uniforms.currentTime.value =
					now;
			}
		}

		const delta = 1 / 60;

		if (temperatureRef.current > 0.1) {
			temperatureRef.current *= coolingRate;
		} else if (isSimulating) {
			setIsSimulating(false);
		}

		simulatorRef.current.simulate(
			delta,
			temperatureRef.current,
			epochMin,
			epochMax,
		);

		if (geometryResultRef.current) {
			updateMaterialTextures(
				geometryResultRef.current,
				simulatorRef.current.getPositionTexture(),
				simulatorRef.current.getNodeAttribTexture(),
				nodeIntensityTextureRef.current,
			);
		}

		controlsRef.current?.update();

		// Keep DoF focused on the orbit target (makes it feel “automatic”).
		if (bokehPassRef.current && controlsRef.current && cameraRef.current) {
			const targetView = controlsRef.current.target
				.clone()
				.applyMatrix4(cameraRef.current.matrixWorldInverse);
			const focus = Math.min(
				cameraRef.current.far,
				Math.max(cameraRef.current.near, -targetView.z),
			);
			const pass = bokehPassRef.current as BokehPass & {
				materialBokeh?: { uniforms?: { focus?: { value: number } } };
			};
			const uniforms = pass.materialBokeh?.uniforms;
			if (uniforms?.focus) uniforms.focus.value = focus;
		}

		if (mouseTrackerRef.current && pickingRef.current) {
			const mouseState = mouseTrackerRef.current.getState();
			// Only update picking when mouse state has changed (down, up, or position changed)
			if (
				mouseState.down ||
				mouseState.up ||
				mouseState.dblClick ||
				mouseState.x !== lastMouseStateRef.current.x ||
				mouseState.y !== lastMouseStateRef.current.y
			) {
				pickingRef.current.update(mouseState);
				lastMouseStateRef.current = mouseState;
			}
		}

		if (composerRef.current && isDofEnabled) {
			if (bokehPassRef.current) bokehPassRef.current.enabled = true;
			composerRef.current.render();
		} else {
			if (bokehPassRef.current) bokehPassRef.current.enabled = false;
			rendererRef.current.render(sceneRef.current, cameraRef.current);
		}
	}, [coolingRate, epochMin, epochMax, isSimulating, isDofEnabled]);

	const handleResize = useCallback(() => {
		if (!containerRef.current || !rendererRef.current || !cameraRef.current)
			return;

		const width = containerRef.current?.clientWidth || window.innerWidth;
		const height = containerRef.current?.clientHeight || window.innerHeight;

		const dpr = window.devicePixelRatio;

		cameraRef.current.aspect = width / height;
		cameraRef.current.updateProjectionMatrix();

		// Match original: manual DPR scaling
		rendererRef.current.setSize(width * dpr, height * dpr, false);
		rendererRef.current.domElement.style.width = `${width}px`;
		rendererRef.current.domElement.style.height = `${height}px`;

		// Keep composer in sync with renderer size
		composerRef.current?.setSize(width * dpr, height * dpr);

		const maybeResizable = bokehPassRef.current as unknown as {
			setSize?: (width: number, height: number) => void;
		};
		if (maybeResizable.setSize) {
			maybeResizable.setSize(width * dpr, height * dpr);
		}

		// Update picking texture size to match renderer
		if (pickingRef.current) {
			pickingRef.current.resize(width, height);
		}
	}, []);

	const applyLayout = useCallback(
		(layoutType: LayoutType) => {
			if (
				!simulatorRef.current ||
				!configRef.current ||
				!geometryResultRef.current
			)
				return;

			const { nodesWidth } = configRef.current;
			const nodesAndEdges = geometryResultRef.current.nodeNames.map((_, i) => [
				i,
			]);
			let layoutTexture: THREE.DataTexture | null = null;

			switch (layoutType) {
				case "dag":
					layoutTexture = generateDagLayoutTexture(
						geometryResultRef.current.nodeNames,
						nodesWidth,
					);
					if (!layoutTexture)
						layoutTexture = generateGridLayout(nodesAndEdges, nodesWidth);
					break;
				case "dag3d":
					layoutTexture = generateDag3dLayoutTexture(
						geometryResultRef.current.nodeNames,
						nodesWidth,
					);
					if (!layoutTexture)
						layoutTexture = generateGridLayout(nodesAndEdges, nodesWidth);
					break;
				case "guided3d":
					layoutTexture = generateGuided3dLayoutTexture(
						geometryResultRef.current.nodeNames,
						nodesWidth,
					);
					if (!layoutTexture)
						layoutTexture = generateGridLayout(nodesAndEdges, nodesWidth);
					break;
				case "radialLayered":
					layoutTexture = generateRadialLayeredLayoutTexture(
						geometryResultRef.current.nodeNames,
						nodesWidth,
					);
					if (!layoutTexture)
						layoutTexture = generateSphericalLayout(nodesAndEdges, nodesWidth);
					break;
				case "radialBfs":
					layoutTexture = generateRadialBfsLayoutTexture(
						geometryResultRef.current.nodeNames,
						nodesWidth,
					);
					if (!layoutTexture)
						layoutTexture = generateSphericalLayout(nodesAndEdges, nodesWidth);
					break;
				case "bfs":
					layoutTexture = generateBfsLayoutTexture(
						geometryResultRef.current.nodeNames,
						nodesWidth,
					);
					if (!layoutTexture)
						layoutTexture = generateGridLayout(nodesAndEdges, nodesWidth);
					break;
				case "bfs3d":
					layoutTexture = generateBfs3dLayoutTexture(
						geometryResultRef.current.nodeNames,
						nodesWidth,
					);
					if (!layoutTexture)
						layoutTexture = generateGridLayout(nodesAndEdges, nodesWidth);
					break;
				case "tree":
					layoutTexture = generateSpanningTreeLayoutTexture(
						geometryResultRef.current.nodeNames,
						nodesWidth,
					);
					if (!layoutTexture)
						layoutTexture = generateGridLayout(nodesAndEdges, nodesWidth);
					break;
				case "tree3d":
					layoutTexture = generateTree3dLayoutTexture(
						geometryResultRef.current.nodeNames,
						nodesWidth,
					);
					if (!layoutTexture)
						layoutTexture = generateGridLayout(nodesAndEdges, nodesWidth);
					break;
				case "layered":
					layoutTexture = generateLayeredLayoutTexture(
						geometryResultRef.current.nodeNames,
						nodesWidth,
					);
					break;
				case "cylinder":
					layoutTexture = generateCylinderLayoutTexture(
						geometryResultRef.current.nodeNames,
						nodesWidth,
					);
					if (!layoutTexture)
						layoutTexture = generateSphericalLayout(nodesAndEdges, nodesWidth);
					break;
				case "circular":
					layoutTexture = generateCircularLayout(nodesAndEdges, nodesWidth);
					break;
				case "spherical":
					layoutTexture = generateSphericalLayout(nodesAndEdges, nodesWidth);
					break;
				case "helix":
					layoutTexture = generateHelixLayout(nodesAndEdges, nodesWidth);
					break;
				case "grid3d":
					layoutTexture = generateGridLayout(nodesAndEdges, nodesWidth);
					break;
				case "grid":
					layoutTexture = generateGridLayout(nodesAndEdges, nodesWidth);
					break;
				default:
					// True force-directed mode: no layout targets (w=0), so the simulator runs n-body.
					layoutTexture = generateZeroedPositionTexture(
						nodesAndEdges,
						nodesWidth,
					);
					break;
			}

			if (layoutTexture) {
				simulatorRef.current.setLayoutPositions(layoutTexture);
				// Configure velocity shader mode. Default (0) keeps deterministic layouts stable.
				// guided3d enables hybrid force+constraints for "native 3D explanatory" layouting.
				if (simulatorRef.current.velocityUniforms.layoutMode) {
					if (layoutType === "guided3d") {
						simulatorRef.current.velocityUniforms.layoutMode.value = 1.0;
						simulatorRef.current.velocityUniforms.layoutStrength.value = 5.0;
						simulatorRef.current.velocityUniforms.layoutMask.value =
							new THREE.Vector3(0, 1, 0);
					} else {
						simulatorRef.current.velocityUniforms.layoutMode.value = 0.0;
						simulatorRef.current.velocityUniforms.layoutStrength.value = 0.0;
						simulatorRef.current.velocityUniforms.layoutMask.value =
							new THREE.Vector3(0, 0, 0);
					}
				}
				const nodeCount = geometryResultRef.current?.nodeNames.length ?? 100;
				temperatureRef.current = nodeCount / 4;
				setIsSimulating(true);
				// Keep OrbitControls centered on the current layout so zoom doesn't clamp around a stale target.
				// Without this, some deterministic layouts can feel "impossible" to zoom into.
				setOrbitTargetToLayoutCenter();
				if (autoFitCamera) setTimeout(() => fitCameraToGraph(), 100);
			}
			setCurrentLayout(layoutType);
			if (typeof window !== "undefined") {
				window.localStorage.setItem(LAYOUT_STORAGE_KEY, layoutType);
			}
		},
		[
			fitCameraToGraph,
			setOrbitTargetToLayoutCenter,
			autoFitCamera,
			generateLayeredLayoutTexture,
			generateCylinderLayoutTexture,
			generateRadialLayeredLayoutTexture,
			generateBfsLayoutTexture,
			generateBfs3dLayoutTexture,
			generateRadialBfsLayoutTexture,
			generateSpanningTreeLayoutTexture,
			generateTree3dLayoutTexture,
			generateDagLayoutTexture,
			generateDag3dLayoutTexture,
			generateGuided3dLayoutTexture,
		],
	);

	useEffect(() => {
		// Re-apply the selected layout after a graph reload (e.g. switching attention/activation).
		void graphVersion;
		if (
			!geometryResultRef.current ||
			!configRef.current ||
			!simulatorRef.current
		)
			return;
		applyLayout(currentLayout);
	}, [applyLayout, currentLayout, graphVersion]);

	const handleTimeChange = useCallback(
		(from: number, to: number) => {
			setTimeRange((prev) => ({ ...prev, from, to }));
			onTimeRangeChange?.(from, to);
		},
		[onTimeRangeChange],
	);

	const stepTimeWindow = useCallback(
		(delta: number) => {
			const span = timeRange.to - timeRange.from;
			const step = Number.isFinite(delta) ? delta : 0;
			if (!Number.isFinite(span) || span < 0) return;

			let nextFrom = timeRange.from + step;
			let nextTo = nextFrom + span;

			// Clamp to [min, max] while preserving span.
			if (nextFrom < timeRange.min) {
				nextFrom = timeRange.min;
				nextTo = nextFrom + span;
			}
			if (nextTo > timeRange.max) {
				nextTo = timeRange.max;
				nextFrom = nextTo - span;
			}

			handleTimeChange(nextFrom, nextTo);
		},
		[
			timeRange.from,
			timeRange.to,
			timeRange.min,
			timeRange.max,
			handleTimeChange,
		],
	);

	const handlePlaybackChange = useCallback(
		(playing: boolean, repeating: boolean, expanded: boolean) => {
			setIsPlaying(playing);
			setIsRepeating(repeating);
			setIsExpanded(expanded);
		},
		[],
	);

	useEffect(() => {
		const cleanup = initScene();
		return () => {
			cancelAnimationFrame(animationFrameRef.current);
			cleanup?.();
		};
	}, [initScene]);

	useEffect(() => {
		animate();
		return () => {
			cancelAnimationFrame(animationFrameRef.current);
		};
	}, [animate]);

	useEffect(() => {
		window.addEventListener("resize", handleResize);
		return () => window.removeEventListener("resize", handleResize);
	}, [handleResize]);

	useEffect(() => {
		const onKeyDown = (e: KeyboardEvent) => {
			if (e.defaultPrevented) return;
			if (e.metaKey || e.ctrlKey || e.altKey) return;

			const target = e.target as HTMLElement | null;
			const tag = target?.tagName?.toLowerCase();
			const isTypingTarget =
				target?.isContentEditable ||
				tag === "input" ||
				tag === "textarea" ||
				tag === "select";
			if (isTypingTarget) return;

			if (e.key === "ArrowLeft") {
				e.preventDefault();
				stepTimeWindow(-1);
			} else if (e.key === "ArrowRight") {
				e.preventDefault();
				stepTimeWindow(1);
			}
		};

		window.addEventListener("keydown", onKeyDown, { passive: false });
		return () => window.removeEventListener("keydown", onKeyDown);
	}, [stepTimeWindow]);

	useEffect(() => {
		if (externalGraph) {
			loadGraph(externalGraph);
		} else {
			const demoGraph = new Graph();
			generators.balancedTree(demoGraph, 7);
			loadGraph(demoGraph);
		}
	}, [externalGraph, loadGraph]);

	useEffect(() => {
		if (geometryResultRef.current) {
			geometryResultRef.current.edgeMesh.visible = showEdges;
			geometryResultRef.current.labelMesh.visible = showLabels;
		}
	}, [showEdges, showLabels]);

	useEffect(() => {
		if (!configRef.current) return;
		const { nodesWidth } = configRef.current;

		if (nodeIntensityTextureRef.current) {
			nodeIntensityTextureRef.current.dispose();
		}

		nodeIntensityTextureRef.current = generateIntensityTexture(
			nodeIntensity,
			nodesWidth,
		);
	}, [nodeIntensity]);

	useEffect(() => {
		applyThreatMask(nodeThreat);
	}, [nodeThreat, applyThreatMask]);

	useEffect(() => {
		const graph = externalGraph;
		const simulator = simulatorRef.current;

		if (!simulator) return;

		if (selectedNodeName == null) {
			simulator.setSelectedNode(-1);
			selectedNodeRef.current = null;
			return;
		}

		const node = graph?.nodes?.[selectedNodeName];

		if (node == null) {
			return;
		}

		simulator.setSelectedNode(node.id);
		selectedNodeRef.current = selectedNodeName;
	}, [selectedNodeName, externalGraph]);

	const formatTime = useCallback(
		(t: number) => {
			// When graph "time" is really a layer index (like the attention visualizer),
			// show layers instead of Jan 1 timestamps.
			const span = timeRange.max - timeRange.min;
			const looksLikeLayerIndex =
				timeRange.min === 0 && Number.isFinite(span) && span > 0 && span <= 512;

			if (looksLikeLayerIndex) {
				return `layer ${Math.round(t) + 1}`;
			}

			// Otherwise treat as epoch seconds relative to epochOffset.
			const epochOffset = configRef.current?.epochOffset ?? 0;
			const absSeconds = epochOffset + t;
			const d = new Date(absSeconds * 1000);
			return d.toLocaleString("en-US", {
				day: "numeric",
				hour: "2-digit",
				minute: "2-digit",
				month: "short",
			});
		},
		[timeRange.max, timeRange.min],
	);

	return (
		<div
			className={`node-graph-legacy flex-1 ${className ?? ""}`}
			style={{
				backgroundColor: "black",
				background:
					"radial-gradient(circle, rgba(60, 60, 60, 1) 0%, rgba(35, 35, 35, 1) 70%, rgba(15, 15, 15, 1) 100%)",
				display: "flex",
				flexDirection: "column",
				fontFamily: "Monospace",
				minHeight: 0,
				overflow: "hidden",
				position: "relative",
				width: "100%",
			}}
		>
			<div
				ref={containerRef}
				style={{ flex: 1, minHeight: 0, width: "100%" }}
			/>

			<NodeGraphLegacyLayoutControls
				autoFitCamera={autoFitCamera}
				currentLayout={currentLayout}
				isDofEnabled={isDofEnabled}
				onApplyLayout={applyLayout}
				onFitCamera={fitCameraToGraph}
				onToggleAutoFit={() => {
					const next = !autoFitCamera;
					setAutoFitCamera(next);
					if (typeof window !== "undefined") {
						window.localStorage.setItem(
							CAMERA_AUTOFIT_STORAGE_KEY,
							next ? "1" : "0",
						);
					}
				}}
				onToggleDof={() => setIsDofEnabled((v) => !v)}
			/>

			<NodeGraphLegacyTimeControls
				formatTime={formatTime}
				isExpanded={isExpanded}
				isPlaying={isPlaying}
				isRepeating={isRepeating}
				onExpandRange={() => sliderRef.current?.expandRange()}
				onTogglePlay={() => sliderRef.current?.togglePlay()}
				onToggleRepeat={() => sliderRef.current?.toggleRepeat()}
				onTimeChange={handleTimeChange}
				onPlaybackChange={handlePlaybackChange}
				showTimeSlider={showTimeSlider}
				sliderRef={sliderRef}
				timeRange={timeRange}
			/>
		</div>
	);
}

export default ModelScope;

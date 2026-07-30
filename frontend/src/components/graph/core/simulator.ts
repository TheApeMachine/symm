/**
 * GPU-based force-directed graph layout simulator
 *
 * Uses GPGPU (General Purpose GPU) computing via Three.js to run
 * force-directed layout calculations on the GPU. This allows for
 * real-time simulation of graphs with thousands of nodes.
 *
 * The simulation uses a "ping-pong" technique with render targets,
 * alternating between two buffers to read from one while writing to another.
 */

import * as THREE from "three";
import {
	nodeAttribSimulationShader,
	passthruFragmentShader,
	passthruVertexShader,
	positionSimulationShader,
	velocitySimulationShader,
} from "../shaders";
import {
	generateDataTexture,
	generateEpochDataTexture,
	generateIdMappings,
	generateIndicesTexture,
	generateNodeAttribTexture,
	generatePositionTexture,
	generateVelocityTexture,
	generateZeroedPositionTexture,
	type NodeAttribConfig,
	type NodeMetrics,
} from "../utils/texture-generators";

export interface SimulatorConfig {
	nodesAndEdges: number[][];
	nodesAndEpochs: number[][];
	nodesWidth: number;
	edgesWidth: number;
	epochsWidth: number;
	epochOffset: number;
	nodeMetrics?: NodeMetrics[]; // Normalized metrics for size/brightness
	nodeAttribConfig?: NodeAttribConfig; // Contrast settings
}

/**
 * GPGPU Simulator for force-directed graph layout
 *
 * Runs physics simulation entirely on the GPU using shader programs.
 * The simulation computes repulsion between all nodes and attraction
 * along edges, with temperature-based cooling for convergence.
 */
export class Simulator {
	private renderer: THREE.WebGLRenderer;
	private camera: THREE.Camera;
	private scene: THREE.Scene;
	private mesh: THREE.Mesh;

	private passThruShader: THREE.ShaderMaterial;
	private velocityShader: THREE.ShaderMaterial;
	private positionShader: THREE.ShaderMaterial;
	private nodeAttribShader: THREE.ShaderMaterial;

	private rtPosition1: THREE.WebGLRenderTarget | null = null;
	private rtPosition2: THREE.WebGLRenderTarget | null = null;
	private rtVelocity1: THREE.WebGLRenderTarget | null = null;
	private rtVelocity2: THREE.WebGLRenderTarget | null = null;
	private rtNodeAttrib1: THREE.WebGLRenderTarget | null = null;
	private rtNodeAttrib2: THREE.WebGLRenderTarget | null = null;

	private flipflop = true;

	// Expose uniforms for external access
	public velocityUniforms: THREE.ShaderMaterial["uniforms"];
	public positionUniforms: THREE.ShaderMaterial["uniforms"];
	public nodeAttribUniforms: THREE.ShaderMaterial["uniforms"];

	constructor(renderer: THREE.WebGLRenderer) {
		this.renderer = renderer;
		this.camera = new THREE.Camera();
		this.camera.position.z = 1;

		this.scene = new THREE.Scene();

		// Passthrough shader for copying textures - defines added in init()
		this.passThruShader = new THREE.ShaderMaterial({
			fragmentShader: passthruFragmentShader,
			uniforms: {
				uTexture: { value: null },
			},
			vertexShader: passthruVertexShader,
		});

		// Create temporary shaders - will be updated in init()
		this.velocityShader = new THREE.ShaderMaterial({
			blending: THREE.NoBlending,
			fragmentShader: velocitySimulationShader,
			uniforms: {
				delta: { value: 0.0 },
				edgeData: { value: null },
				edgeIndices: { value: null },
				k: { value: 400.0 },
				layoutMask: { value: new THREE.Vector3(0, 0, 0) },
				layoutMode: { value: 0.0 },
				layoutPositions: { value: null },
				layoutStrength: { value: 0.0 },
				positions: { value: null },
				temperature: { value: 0.0 },
				velocities: { value: null },
			},
			vertexShader: passthruVertexShader,
		});

		this.positionShader = new THREE.ShaderMaterial({
			blending: THREE.NoBlending,
			fragmentShader: positionSimulationShader,
			uniforms: {
				delta: { value: 0.0 },
				positions: { value: null },
				temperature: { value: 0.0 },
				velocities: { value: null },
			},
			vertexShader: passthruVertexShader,
		});

		this.nodeAttribShader = new THREE.ShaderMaterial({
			blending: THREE.NoBlending,
			fragmentShader: nodeAttribSimulationShader,
			uniforms: {
				delta: { value: 0.0 },
				edgeData: { value: null },
				edgeIndices: { value: null },
				epochsData: { value: null },
				epochsIndices: { value: null },
				hoverMode: { value: 1.0 },
				maxTime: { value: 0.0 },
				minTime: { value: 0.0 },
				nodeAttrib: { value: null },
				nodeIDMappings: { value: null },
				selectedNode: { value: -1.0 },
			},
			vertexShader: passthruVertexShader,
		});

		this.velocityUniforms = this.velocityShader.uniforms;
		this.positionUniforms = this.positionShader.uniforms;
		this.nodeAttribUniforms = this.nodeAttribShader.uniforms;

		// Create mesh for rendering
		const geometry = new THREE.PlaneGeometry(2, 2);
		this.mesh = new THREE.Mesh(geometry, this.passThruShader);
		this.scene.add(this.mesh);
	}

	/**
	 * Initialize the simulator with graph data
	 *
	 * Creates all necessary textures and render targets for the simulation.
	 */
	init(config: SimulatorConfig): void {
		const {
			nodesAndEdges,
			nodesAndEpochs,
			nodesWidth,
			edgesWidth,
			epochsWidth,
			epochOffset,
			nodeMetrics,
			nodeAttribConfig,
		} = config;

		// Update shader defines
		const nodesWidthStr = nodesWidth.toFixed(2);
		const edgesWidthStr = edgesWidth.toFixed(2);
		const epochsWidthStr = epochsWidth.toFixed(2);

		// Recreate shaders with proper defines
		// Important: passthruFragmentShader requires NODESWIDTH

		// Add NODESWIDTH define to passthrough shader (required by fs-passthru)
		this.passThruShader.defines = {
			NODESWIDTH: nodesWidthStr,
		};
		this.passThruShader.needsUpdate = true;

		this.velocityShader.defines = {
			EDGESWIDTH: edgesWidthStr,
			NODESWIDTH: nodesWidthStr,
		};
		this.velocityShader.needsUpdate = true;

		this.positionShader.defines = {
			NODESWIDTH: nodesWidthStr,
		};
		this.positionShader.needsUpdate = true;

		this.nodeAttribShader.defines = {
			EDGESWIDTH: edgesWidthStr,
			EPOCHSWIDTH: epochsWidthStr,
			NODESWIDTH: nodesWidthStr,
		};
		this.nodeAttribShader.needsUpdate = true;

		// Generate initial textures
		const dtPosition = generatePositionTexture(nodesAndEdges, nodesWidth, 1000);
		const dtVelocity = generateVelocityTexture(nodesAndEdges, nodesWidth);
		// Pass node metrics and config for size/brightness variation
		const dtNodeAttrib = generateNodeAttribTexture(
			nodesAndEdges,
			nodesWidth,
			nodeMetrics,
			nodeAttribConfig,
		);

		// Set up velocity shader textures
		this.velocityUniforms.edgeIndices.value = generateIndicesTexture(
			nodesAndEdges,
			nodesWidth,
		);
		this.velocityUniforms.edgeData.value = generateDataTexture(
			nodesAndEdges,
			edgesWidth,
		);
		this.velocityUniforms.layoutPositions.value = generateZeroedPositionTexture(
			nodesAndEdges,
			edgesWidth,
		);

		// Set up node attrib shader textures
		this.nodeAttribUniforms.epochsIndices.value = generateIndicesTexture(
			nodesAndEpochs,
			nodesWidth,
		);
		this.nodeAttribUniforms.epochsData.value = generateEpochDataTexture(
			nodesAndEpochs,
			epochsWidth,
			epochOffset,
		);
		this.nodeAttribUniforms.edgeIndices.value =
			this.velocityUniforms.edgeIndices.value;
		this.nodeAttribUniforms.edgeData.value =
			this.velocityUniforms.edgeData.value;
		this.nodeAttribUniforms.nodeIDMappings.value = generateIdMappings(
			nodesAndEpochs,
			nodesWidth,
		);

		// Create render targets
		this.rtPosition1 = this.createRenderTarget(nodesWidth);
		this.rtPosition2 = this.rtPosition1.clone();
		this.rtVelocity1 = this.createRenderTarget(nodesWidth);
		this.rtVelocity2 = this.rtVelocity1.clone();
		this.rtNodeAttrib1 = this.createRenderTarget(nodesWidth);
		this.rtNodeAttrib2 = this.rtNodeAttrib1.clone();

		// Initialize render targets with initial data
		this.renderTexture(dtPosition, this.rtPosition1);
		this.renderTexture(this.rtPosition1.texture, this.rtPosition2);
		this.renderTexture(dtVelocity, this.rtVelocity1);
		this.renderTexture(this.rtVelocity1.texture, this.rtVelocity2);
		this.renderTexture(dtNodeAttrib, this.rtNodeAttrib1);
		this.renderTexture(this.rtNodeAttrib1.texture, this.rtNodeAttrib2);
	}

	private createRenderTarget(size: number): THREE.WebGLRenderTarget {
		return new THREE.WebGLRenderTarget(size, size, {
			format: THREE.RGBAFormat,
			magFilter: THREE.NearestFilter,
			minFilter: THREE.NearestFilter,
			stencilBuffer: false,
			type: THREE.FloatType,
			wrapS: THREE.RepeatWrapping,
			wrapT: THREE.RepeatWrapping,
		});
	}

	private renderTexture(
		input: THREE.Texture | THREE.WebGLRenderTarget,
		output: THREE.WebGLRenderTarget,
	): void {
		this.mesh.material = this.passThruShader;
		this.passThruShader.uniforms.uTexture.value =
			input instanceof THREE.WebGLRenderTarget ? input.texture : input;

		this.renderer.setRenderTarget(output);
		this.renderer.render(this.scene, this.camera);
		this.renderer.setRenderTarget(null);
	}

	private renderVelocity(
		position: THREE.WebGLRenderTarget,
		velocity: THREE.WebGLRenderTarget,
		output: THREE.WebGLRenderTarget,
		delta: number,
		temperature: number,
	): void {
		this.mesh.material = this.velocityShader;
		this.velocityUniforms.positions.value = position.texture;
		this.velocityUniforms.velocities.value = velocity.texture;
		this.velocityUniforms.temperature.value = temperature;
		this.velocityUniforms.delta.value = delta;
		this.renderer.setRenderTarget(output);
		this.renderer.render(this.scene, this.camera);
		this.renderer.setRenderTarget(null);
	}

	private renderPosition(
		position: THREE.WebGLRenderTarget,
		velocity: THREE.WebGLRenderTarget,
		output: THREE.WebGLRenderTarget,
		delta: number,
	): void {
		this.mesh.material = this.positionShader;
		this.positionUniforms.positions.value = position.texture;
		this.positionUniforms.velocities.value = velocity.texture;
		this.positionUniforms.delta.value = delta;
		this.renderer.setRenderTarget(output);
		this.renderer.render(this.scene, this.camera);
		this.renderer.setRenderTarget(null);
	}

	private renderNodeAttrib(
		nodeAttrib: THREE.WebGLRenderTarget,
		output: THREE.WebGLRenderTarget,
		epochMin: number,
		epochMax: number,
		delta: number,
	): void {
		this.mesh.material = this.nodeAttribShader;
		this.nodeAttribUniforms.nodeAttrib.value = nodeAttrib.texture;
		this.nodeAttribUniforms.minTime.value = epochMin;
		this.nodeAttribUniforms.maxTime.value = epochMax;
		this.nodeAttribUniforms.delta.value = delta;
		this.renderer.setRenderTarget(output);
		this.renderer.render(this.scene, this.camera);
		this.renderer.setRenderTarget(null);
	}

	/**
	 * Run one step of the simulation
	 *
	 * Updates velocities based on forces, then updates positions based on velocities.
	 * Uses ping-pong buffers to alternate between read and write targets.
	 */
	simulate(
		delta: number,
		temperature: number,
		epochMin: number,
		epochMax: number,
	): void {
		if (
			!this.rtPosition1 ||
			!this.rtPosition2 ||
			!this.rtVelocity1 ||
			!this.rtVelocity2 ||
			!this.rtNodeAttrib1 ||
			!this.rtNodeAttrib2
		) {
			return;
		}

		if (this.flipflop) {
			if (temperature > 0.1) {
				this.renderVelocity(
					this.rtPosition1,
					this.rtVelocity1,
					this.rtVelocity2,
					delta,
					temperature,
				);
				this.renderPosition(
					this.rtPosition1,
					this.rtVelocity2,
					this.rtPosition2,
					delta,
				);
			}
			this.renderNodeAttrib(
				this.rtNodeAttrib1,
				this.rtNodeAttrib2,
				epochMin,
				epochMax,
				delta,
			);
		} else {
			if (temperature > 0.1) {
				this.renderVelocity(
					this.rtPosition2,
					this.rtVelocity2,
					this.rtVelocity1,
					delta,
					temperature,
				);
				this.renderPosition(
					this.rtPosition2,
					this.rtVelocity1,
					this.rtPosition1,
					delta,
				);
			}
			this.renderNodeAttrib(
				this.rtNodeAttrib2,
				this.rtNodeAttrib1,
				epochMin,
				epochMax,
				delta,
			);
		}

		this.flipflop = !this.flipflop;
	}

	/**
	 * Get current position texture for rendering
	 *
	 * Note: simulate() toggles flipflop at the end, so we need the OPPOSITE
	 * of what flipflop indicates to get the most recently written texture.
	 */
	getPositionTexture(): THREE.Texture | null {
		// After simulate(), flipflop has been toggled, so:
		// - If flipflop is now false, simulate just wrote to rtPosition2
		// - If flipflop is now true, simulate just wrote to rtPosition1
		if (this.flipflop) {
			return this.rtPosition1?.texture ?? null;
		}
		return this.rtPosition2?.texture ?? null;
	}

	/**
	 * Get current node attribute texture for rendering
	 */
	getNodeAttribTexture(): THREE.Texture | null {
		// Same logic as getPositionTexture
		if (this.flipflop) {
			return this.rtNodeAttrib1?.texture ?? null;
		}
		return this.rtNodeAttrib2?.texture ?? null;
	}

	/**
	 * Set layout positions for animated transitions
	 */
	setLayoutPositions(texture: THREE.DataTexture): void {
		this.velocityUniforms.layoutPositions.value = texture;
	}

	/**
	 * Set selected node for highlighting
	 */
	setSelectedNode(nodeId: number): void {
		this.nodeAttribUniforms.selectedNode.value = nodeId;
	}

	/**
	 * Set hover mode (1 = hover highlight, 0 = click select)
	 */
	setHoverMode(mode: number): void {
		this.nodeAttribUniforms.hoverMode.value = mode;
	}

	/**
	 * Dispose of all GPU resources
	 */
	dispose(): void {
		this.rtPosition1?.dispose();
		this.rtPosition2?.dispose();
		this.rtVelocity1?.dispose();
		this.rtVelocity2?.dispose();
		this.rtNodeAttrib1?.dispose();
		this.rtNodeAttrib2?.dispose();
		this.passThruShader.dispose();
		this.velocityShader.dispose();
		this.positionShader.dispose();
		this.nodeAttribShader.dispose();
	}
}

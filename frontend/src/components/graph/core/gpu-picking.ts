/**
 * GPU-based picking for node selection
 *
 * Uses a separate render target with unique colors per node to identify
 * which node is under the mouse cursor. This allows efficient picking
 * even with thousands of nodes.
 */

import * as THREE from "three";
import type { Simulator } from "./simulator";

export interface PickingState {
	selectedNode: number | null;
	hoveredNode: number;
	nodeClicked: { down: number | null; up: number | null };
}

export interface MouseState {
	x: number;
	y: number;
	down: boolean;
	up: boolean;
	dblClick: boolean;
}

/**
 * GPU Picking system for node selection
 *
 * Renders nodes to an offscreen buffer with unique colors encoding node IDs.
 * Reading the pixel under the mouse reveals which node is being pointed at.
 */
export class GPUPicking {
	private renderer: THREE.WebGLRenderer;
	private pickingScene: THREE.Scene;
	private camera: THREE.Camera;
	private simulator: Simulator;

	private pickingTexture: THREE.WebGLRenderTarget;
	private pixelBuffer: Uint8Array;

	private state: PickingState = {
		hoveredNode: -1,
		nodeClicked: { down: null, up: null },
		selectedNode: null,
	};

	private lastHoveredNode = -1;

	// Callbacks
	public onNodeSelect?: (nodeIndex: number, nodeName: string) => void;
	public onNodeHover?: (nodeIndex: number, nodeName: string) => void;
	public onSelectionClear?: () => void;

	// Node name lookup
	private nodeNames: string[] = [];

	constructor(
		renderer: THREE.WebGLRenderer,
		pickingScene: THREE.Scene,
		camera: THREE.Camera,
		simulator: Simulator,
	) {
		this.renderer = renderer;
		this.pickingScene = pickingScene;
		this.camera = camera;
		this.simulator = simulator;

		this.pickingTexture = new THREE.WebGLRenderTarget();

		this.pickingTexture.texture.minFilter = THREE.NearestFilter;
		this.pickingTexture.texture.magFilter = THREE.NearestFilter;
		this.pickingTexture.texture.generateMipmaps = false;

		this.pixelBuffer = new Uint8Array(4);
	}

	/**
	 * Set the node names for lookup
	 */
	setNodeNames(names: string[]): void {
		this.nodeNames = names;
	}

	/**
	 * Resize the picking render target to match renderer size
	 * width/height are CSS pixels - we multiply by DPR like the original does
	 */
	resize(width: number, height: number): void {
		const dpr = window.devicePixelRatio;
		const pixelWidth = Math.floor(width * dpr);
		const pixelHeight = Math.floor(height * dpr);
		this.pickingTexture.setSize(pixelWidth, pixelHeight);
		// Store CSS dimensions for coordinate calculation
		this.cssWidth = width;
		this.cssHeight = height;
		console.log(
			"[GPUPicking] Resized to:",
			pixelWidth,
			"x",
			pixelHeight,
			"(CSS:",
			width,
			"x",
			height,
			", DPR:",
			dpr,
			")",
		);
	}

	// Store CSS dimensions for proper coordinate mapping
	private cssWidth = 0;
	private cssHeight = 0;

	/**
	 * Get current selected node
	 */
	getSelectedNode(): number | null {
		return this.state.selectedNode;
	}

	/**
	 * Get current hovered node
	 */
	getHoveredNode(): number {
		return this.state.hoveredNode;
	}

	/**
	 * Clear selection
	 */
	clearSelection(): void {
		this.state.selectedNode = null;
		this.simulator.setHoverMode(1);
		this.onSelectionClear?.();
	}

	// Debug flag - set to true to render picking scene to screen
	private debugRenderPickingScene = false;

	/**
	 * Update picking state
	 *
	 * Renders to picking buffer and determines which node is under the mouse.
	 * Handles click and hover events.
	 */
	update(mouse: MouseState): void {
		// Render picking scene
		const currentClearColor = this.renderer.getClearColor(new THREE.Color());
		const currentClearAlpha = this.renderer.getClearAlpha();

		this.renderer.setClearColor(0);

		if (this.debugRenderPickingScene) {
			// Debug: Render to screen instead of texture to visualize picking scene
			this.renderer.setRenderTarget(null);
			this.renderer.clear();
			this.renderer.render(this.pickingScene, this.camera);
			return; // Skip actual picking in debug mode
		}

		this.renderer.setRenderTarget(this.pickingTexture);
		this.renderer.clear(); // Explicitly clear the render target
		this.renderer.render(this.pickingScene, this.camera);

		// Create buffer for reading single pixel - match example
		this.pixelBuffer = new Uint8Array(4);

		// Read the pixel under the mouse from the texture - match example exactly
		const dpr = window.devicePixelRatio;
		// Floor the coordinates since readRenderTargetPixels expects integers
		const pixelX = Math.floor(mouse.x * dpr);
		const pixelY = Math.floor(this.pickingTexture.height - mouse.y * dpr - 1);

		// Debug: Log coordinates on click
		if (mouse.down) {
			console.log("[GPUPicking] Debug coordinates:", {
				dpr,
				mouseCSS: { x: mouse.x, y: mouse.y },
				pixelCoords: { x: pixelX, y: pixelY },
				storedCSSSize: {
					h: this.cssHeight,
					w: this.cssWidth,
				},
				textureSize: {
					h: this.pickingTexture.height,
					w: this.pickingTexture.width,
				},
			});
		}

		// Bounds check - must restore state before returning!
		if (
			pixelX < 0 ||
			pixelX >= this.pickingTexture.width ||
			pixelY < 0 ||
			pixelY >= this.pickingTexture.height
		) {
			if (mouse.down) {
				console.log("[GPUPicking] Out of bounds!");
			}
			// Restore render target and clear color before returning
			this.renderer.setRenderTarget(null);
			this.renderer.setClearColor(currentClearColor, currentClearAlpha);
			return;
		}

		this.renderer.readRenderTargetPixels(
			this.pickingTexture,
			pixelX,
			pixelY,
			1,
			1,
			this.pixelBuffer,
		);

		const color =
			(this.pixelBuffer[0] << 16) |
			(this.pixelBuffer[1] << 8) |
			this.pixelBuffer[2];
		const id = color - 1;

		// Handle mouse down
		if (this.state.nodeClicked.down === null && mouse.down) {
			this.state.nodeClicked.down = id;
		}

		// Handle mouse up
		if (
			this.state.nodeClicked.down !== null &&
			this.state.nodeClicked.up === null &&
			mouse.up
		) {
			this.state.nodeClicked.up = id;
			this.handleClick();
		}

		// Handle hover (only when no selection and not clicking)
		if (
			this.state.selectedNode === null &&
			this.state.nodeClicked.down === null
		) {
			if (this.lastHoveredNode !== id) {
				this.lastHoveredNode = id;
				this.state.hoveredNode = id;
				this.handleHover(id);
			}
		}

		// Restore render target
		this.renderer.setRenderTarget(null);
		this.renderer.setClearColor(currentClearColor, currentClearAlpha);

		// Handle double click to clear selection
		if (mouse.dblClick) {
			this.clearSelection();
		}
	}

	private handleClick(): void {
		const { down, up } = this.state.nodeClicked;

		// Only select if mouse down and up on same node
		if (down === up && down !== null && down >= 0) {
			this.state.selectedNode = down;
			this.simulator.setSelectedNode(down);
			this.simulator.setHoverMode(0);

			const nodeName = this.nodeNames[down] ?? String(down);
			console.log("[GPUPicking] Selected node:", down, nodeName);
			this.onNodeSelect?.(down, nodeName);
		}

		// Reset click state
		this.state.nodeClicked.down = null;
		this.state.nodeClicked.up = null;
	}

	private handleHover(id: number): void {
		// Just set the selected node for hover highlighting
		// hoverMode stays at 1 (default) during hover - only changes to 0 on click
		this.simulator.setSelectedNode(id);

		if (id >= 0) {
			const nodeName = this.nodeNames[id] ?? String(id);
			this.onNodeHover?.(id, nodeName);
		}
	}

	/**
	 * Dispose of resources
	 */
	dispose(): void {
		this.pickingTexture.dispose();
	}
}

/**
 * Mouse state tracker
 *
 * Tracks mouse position and button states for picking.
 */
export class MouseTracker {
	private state: MouseState = {
		dblClick: false,
		down: false,
		up: false,
		x: 0,
		y: 0,
	};

	private element: HTMLElement;
	private boundHandlers: {
		move: (e: MouseEvent) => void;
		down: (e: MouseEvent) => void;
		up: (e: MouseEvent) => void;
		dblClick: (e: MouseEvent) => void;
	};

	constructor(element: HTMLElement) {
		this.element = element;

		this.boundHandlers = {
			dblClick: this.handleDblClick.bind(this),
			down: this.handleMouseDown.bind(this),
			move: this.handleMouseMove.bind(this),
			up: this.handleMouseUp.bind(this),
		};

		element.addEventListener("mousemove", this.boundHandlers.move);
		element.addEventListener("mousedown", this.boundHandlers.down);
		element.addEventListener("mouseup", this.boundHandlers.up);
		element.addEventListener("dblclick", this.boundHandlers.dblClick);
	}

	private updatePosition(e: MouseEvent): void {
		const rect = this.element.getBoundingClientRect();
		this.state.x = e.clientX - rect.left;
		this.state.y = e.clientY - rect.top;
	}

	private handleMouseMove(e: MouseEvent): void {
		this.updatePosition(e);
	}

	private handleMouseDown(e: MouseEvent): void {
		if (e.button === 0) {
			this.updatePosition(e);
			this.state.down = true;
		}
	}

	private handleMouseUp(e: MouseEvent): void {
		if (e.button === 0) {
			this.updatePosition(e);
			this.state.up = true;
		}
	}

	private handleDblClick(e: MouseEvent): void {
		this.updatePosition(e);
		this.state.dblClick = true;
	}

	/**
	 * Get current mouse state
	 *
	 * Returns current state and resets transient flags (down, up, dblClick)
	 */
	getState(): MouseState {
		const currentState = { ...this.state };

		// Reset transient states
		this.state.down = false;
		this.state.up = false;
		this.state.dblClick = false;

		return currentState;
	}

	/**
	 * Dispose of event listeners
	 */
	dispose(): void {
		this.element.removeEventListener("mousemove", this.boundHandlers.move);
		this.element.removeEventListener("mousedown", this.boundHandlers.down);
		this.element.removeEventListener("mouseup", this.boundHandlers.up);
		this.element.removeEventListener("dblclick", this.boundHandlers.dblClick);
	}
}

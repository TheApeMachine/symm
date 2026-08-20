import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { type FluidFieldOptions, FluidFieldView } from "./fields";
import { FluidParticles } from "./particles";
import type { FluidFields, FluidParticle, FluidParticleFrame } from "./wire";

export type FluidSceneOptions = FluidFieldOptions & {
	particles: boolean;
};

/*
FluidScene owns the Three.js resources for the interactive inspector. Incoming
domain values update GPU buffers and textures directly through the two views.
*/
export class FluidScene {
	private readonly scene = new THREE.Scene();
	private readonly camera = new THREE.PerspectiveCamera(48, 1, 0.001, 20);
	private readonly renderer: THREE.WebGLRenderer;
	private readonly controls: OrbitControls;
	private readonly fields = new FluidFieldView();
	private readonly particles = new FluidParticles();
	private readonly raycaster = new THREE.Raycaster();
	private readonly boundaryGeometry: THREE.EdgesGeometry;
	private readonly boundaryMaterial: THREE.LineBasicMaterial;
	private readonly resizeObserver: ResizeObserver;
	private animationFrame = 0;
	private invalidated = false;
	private gridSpacing = 1 / 64;

	constructor(
		private readonly container: HTMLElement,
		private readonly onSelect: (particle: FluidParticle | null) => void,
	) {
		this.renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false });
		this.renderer.setClearColor(0x0e0c0a, 1);
		this.renderer.outputColorSpace = THREE.SRGBColorSpace;
		this.renderer.domElement.className = "block size-full touch-none";
		this.container.append(this.renderer.domElement);
		this.camera.position.set(1.65, 1.35, 1.65);
		this.controls = new OrbitControls(this.camera, this.renderer.domElement);
		this.controls.target.setScalar(0.5);
		this.controls.enableDamping = true;
		this.controls.addEventListener("change", this.invalidate);
		this.scene.add(this.fields.group, this.particles.points);
		const boundary = this.createBoundary();
		this.boundaryGeometry = boundary.geometry;
		this.boundaryMaterial = boundary.material;
		this.scene.add(boundary);
		this.renderer.domElement.addEventListener("pointerup", this.pick);
		this.resizeObserver = new ResizeObserver(this.resize);
		this.resizeObserver.observe(this.container);
		this.resize();
		this.invalidate();
	}

	updateFields(fields: FluidFields) {
		this.fields.update(fields);
		this.gridSpacing = fields.grid.spacing;
		this.particles.setGridSpacing(fields.grid.spacing);
		this.invalidate();
	}

	updateParticles(particles: FluidParticleFrame) {
		this.particles.update(particles);
		this.invalidate();
	}

	setOptions(options: FluidSceneOptions) {
		this.fields.setOptions(options);
		this.particles.points.visible = options.particles;
		this.invalidate();
	}

	setSlices(x: number, y: number, z: number) {
		this.fields.setSlices(x, y, z);
		this.invalidate();
	}

	dispose() {
		cancelAnimationFrame(this.animationFrame);
		this.resizeObserver.disconnect();
		this.renderer.domElement.removeEventListener("pointerup", this.pick);
		this.controls.removeEventListener("change", this.invalidate);
		this.controls.dispose();
		this.fields.dispose();
		this.particles.dispose();
		this.boundaryGeometry.dispose();
		this.boundaryMaterial.dispose();
		this.renderer.dispose();
		this.renderer.domElement.remove();
	}

	private readonly resize = () => {
		const width = this.container.clientWidth;
		const height = this.container.clientHeight;

		if (width === 0 || height === 0) {
			return;
		}

		this.renderer.setPixelRatio(window.devicePixelRatio);
		this.renderer.setSize(width, height, false);
		this.camera.aspect = width / height;
		this.camera.updateProjectionMatrix();
		const projectionScale =
			height *
			window.devicePixelRatio *
			0.5 *
			this.camera.projectionMatrix.elements[5];
		this.particles.setProjectionScale(projectionScale);
		this.invalidate();
	};

	private readonly render = () => {
		this.animationFrame = 0;
		this.invalidated = false;
		const controlsChanged = this.controls.update();
		this.renderer.render(this.scene, this.camera);

		if (controlsChanged || this.invalidated) {
			this.animationFrame = requestAnimationFrame(this.render);
		}
	};

	private readonly invalidate = () => {
		this.invalidated = true;

		if (this.animationFrame === 0) {
			this.animationFrame = requestAnimationFrame(this.render);
		}
	};

	private readonly pick = (event: PointerEvent) => {
		if (!this.particles.points.visible) {
			return;
		}

		const bounds = this.renderer.domElement.getBoundingClientRect();
		const pointer = new THREE.Vector2(
			((event.clientX - bounds.left) / bounds.width) * 2 - 1,
			-((event.clientY - bounds.top) / bounds.height) * 2 + 1,
		);
		this.raycaster.params.Points = { threshold: this.gridSpacing };
		this.raycaster.setFromCamera(pointer, this.camera);
		const intersection = this.raycaster.intersectObject(
			this.particles.points,
			false,
		)[0];
		this.onSelect(
			intersection?.index === undefined
				? null
				: this.particles.particle(intersection.index),
		);
	};

	private createBoundary() {
		const box = new THREE.BoxGeometry(1, 1, 1);
		const geometry = new THREE.EdgesGeometry(box);
		box.dispose();
		const material = new THREE.LineBasicMaterial({
			color: 0x544d43,
			transparent: true,
			opacity: 0.7,
		});
		const boundary = new THREE.LineSegments(geometry, material);
		boundary.position.setScalar(0.5);
		return boundary;
	}
}

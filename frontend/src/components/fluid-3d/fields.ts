import * as THREE from "three";
import {
	fluidFieldVertexShader,
	fluidSliceFragmentShader,
	fluidVolumeFragmentShader,
} from "./field-shaders";
import {
	createFluidFieldTextures,
	type FluidFieldTextures,
	updateFluidFieldTextures,
} from "./field-textures";
import type { FluidFields } from "./wire";

export type FluidFieldOptions = {
	gas: boolean;
	wave: boolean;
	volume: boolean;
	slices: boolean;
	exposure: number;
};

type FieldUniforms = ReturnType<typeof fieldUniforms>;

const fieldUniforms = (textures: FluidFieldTextures) => ({
	uMomRho: { value: textures.momRho },
	uInternalEnergy: { value: textures.internalEnergy },
	uWaveReal: { value: textures.waveReal },
	uWaveImaginary: { value: textures.waveImaginary },
	uDensityScale: { value: textures.densityScale },
	uMomentumScale: { value: textures.momentumScale },
	uEnergyScale: { value: textures.energyScale },
	uWaveScale: { value: textures.waveScale },
	uGrid: {
		value: new THREE.Vector3(textures.grid.x, textures.grid.y, textures.grid.z),
	},
	uShowGas: { value: true },
	uShowWave: { value: true },
	uExposure: { value: 1.5 },
});

const volumeMaterial = (uniforms: FieldUniforms) =>
	new THREE.ShaderMaterial({
		glslVersion: THREE.GLSL3,
		uniforms,
		vertexShader: fluidFieldVertexShader,
		fragmentShader: fluidVolumeFragmentShader,
		transparent: true,
		depthWrite: false,
		side: THREE.BackSide,
		blending: THREE.AdditiveBlending,
	});

const sliceMaterial = (uniforms: FieldUniforms) =>
	new THREE.ShaderMaterial({
		glslVersion: THREE.GLSL3,
		uniforms,
		vertexShader: fluidFieldVertexShader,
		fragmentShader: fluidSliceFragmentShader,
		transparent: true,
		depthWrite: false,
		side: THREE.DoubleSide,
		blending: THREE.AdditiveBlending,
	});

/*
FluidFieldView renders the resident Eulerian gas and complex wave arrays as a
raymarched unit volume and three independently movable diagnostic slices.
*/
export class FluidFieldView {
	readonly group = new THREE.Group();
	private textures: FluidFieldTextures | null = null;
	private uniforms: FieldUniforms | null = null;
	private volume: THREE.Mesh | null = null;
	private sliceGroup: THREE.Group | null = null;
	private volumeGeometry: THREE.BoxGeometry | null = null;
	private sliceGeometry: THREE.PlaneGeometry | null = null;
	private volumeShader: THREE.ShaderMaterial | null = null;
	private sliceShader: THREE.ShaderMaterial | null = null;
	private options: FluidFieldOptions = {
		gas: true,
		wave: true,
		volume: true,
		slices: false,
		exposure: 1.5,
	};

	update(fields: FluidFields) {
		if (this.textures !== null) {
			updateFluidFieldTextures(this.textures, fields);
			this.refreshUniforms(this.textures);
			this.applyOptions();
			return;
		}

		const textures = createFluidFieldTextures(fields);
		this.build(textures);
		this.textures = textures;
		this.applyOptions();
	}

	setOptions(options: FluidFieldOptions) {
		this.options = options;
		this.applyOptions();
	}

	setSlices(x: number, y: number, z: number) {
		if (this.sliceGroup === null) {
			return;
		}

		this.sliceGroup.children[0]?.position.set(x, 0.5, 0.5);
		this.sliceGroup.children[1]?.position.set(0.5, y, 0.5);
		this.sliceGroup.children[2]?.position.set(0.5, 0.5, z);
	}

	dispose() {
		this.textures?.dispose();
		this.volumeGeometry?.dispose();
		this.sliceGeometry?.dispose();
		this.volumeShader?.dispose();
		this.sliceShader?.dispose();
		this.group.clear();
	}

	private build(textures: FluidFieldTextures) {
		this.uniforms = fieldUniforms(textures);
		this.volumeGeometry = new THREE.BoxGeometry(1, 1, 1);
		this.volumeShader = volumeMaterial(this.uniforms);
		this.volume = new THREE.Mesh(this.volumeGeometry, this.volumeShader);
		this.volume.position.setScalar(0.5);
		this.volume.renderOrder = 0;
		this.group.add(this.volume);

		this.sliceGeometry = new THREE.PlaneGeometry(1, 1);
		this.sliceShader = sliceMaterial(this.uniforms);
		this.sliceGroup = new THREE.Group();
		const xSlice = new THREE.Mesh(this.sliceGeometry, this.sliceShader);
		xSlice.rotation.y = Math.PI / 2;
		const ySlice = new THREE.Mesh(this.sliceGeometry, this.sliceShader);
		ySlice.rotation.x = -Math.PI / 2;
		const zSlice = new THREE.Mesh(this.sliceGeometry, this.sliceShader);
		this.sliceGroup.add(xSlice, ySlice, zSlice);

		for (const slice of this.sliceGroup.children) {
			slice.renderOrder = 1;
		}

		this.group.add(this.sliceGroup);
		this.setSlices(0.5, 0.5, 0.5);
	}

	private refreshUniforms(textures: FluidFieldTextures) {
		const uniforms = this.uniforms;

		if (uniforms === null) {
			return;
		}

		uniforms.uMomRho.value = textures.momRho;
		uniforms.uInternalEnergy.value = textures.internalEnergy;
		uniforms.uWaveReal.value = textures.waveReal;
		uniforms.uWaveImaginary.value = textures.waveImaginary;
		uniforms.uDensityScale.value = textures.densityScale;
		uniforms.uMomentumScale.value = textures.momentumScale;
		uniforms.uEnergyScale.value = textures.energyScale;
		uniforms.uWaveScale.value = textures.waveScale;
		uniforms.uGrid.value.set(textures.grid.x, textures.grid.y, textures.grid.z);
	}

	private applyOptions() {
		if (this.uniforms !== null) {
			this.uniforms.uShowGas.value = this.options.gas;
			this.uniforms.uShowWave.value = this.options.wave;
			this.uniforms.uExposure.value = this.options.exposure;
		}

		if (this.volume !== null) {
			this.volume.visible = this.options.volume;
		}

		if (this.sliceGroup !== null) {
			this.sliceGroup.visible = this.options.slices;
		}
	}
}

import { currentShader } from "./field-shaders";
import { createVertexBuffer, type FluidGPU } from "./gpu";
import type { FluidFields } from "./wire";

// Display resource budget: at most 16³ spatial glyphs, independent of RAF rate.
const MAX_CURRENT_AXIS_SAMPLES = 16;

/* Sample Im(conj(Ψ) ∇Ψ) at cell midpoints, using the same periodic trilinear
complex interpolation as the pilot solver. This is phase flux; a particle's
velocity additionally needs hbar / (mass * |Ψ|²). No speed or time is invented.
The Z-fastest wire layout is x*ny*nz + y*nz + z. */
export const phaseCurrentGlyphs = (fields: FluidFields) => {
	const dimensions = [fields.grid.x, fields.grid.y, fields.grid.z];
	const samples = dimensions.map((size) =>
		Math.min(size, MAX_CURRENT_AXIS_SAMPLES),
	);
	const count = samples[0] * samples[1] * samples[2];
	const vectors = new Float64Array(count * 3);
	const positions = new Float64Array(count * 3);
	let peak = 0;
	for (let sample = 0; sample < count; sample++) {
		const tile = [
			Math.floor(sample / (samples[1] * samples[2])),
			Math.floor(sample / samples[2]) % samples[1],
			sample % samples[2],
		];
		const base = tile.map((value, axis) =>
			Math.floor(((value + 0.5) * dimensions[axis]) / samples[axis]),
		);
		let real = 0;
		let imaginary = 0;
		const realGradient = [0, 0, 0];
		const imaginaryGradient = [0, 0, 0];
		const cornerValues: Array<[number, number]> = [];
		for (let corner = 0; corner < 8; corner++) {
			const offset = [corner >> 2, (corner >> 1) & 1, corner & 1];
			const cell =
				((base[0] + offset[0]) % dimensions[0]) *
					dimensions[1] *
					dimensions[2] +
				((base[1] + offset[1]) % dimensions[1]) * dimensions[2] +
				((base[2] + offset[2]) % dimensions[2]);
			const cornerReal = fields.waveReal[cell];
			const cornerImaginary = fields.waveImaginary[cell];
			real += cornerReal / 8;
			imaginary += cornerImaginary / 8;
			cornerValues.push([cornerReal, cornerImaginary]);
		}
		for (let axis = 0; axis < 3; axis++) {
			const mask = 1 << (2 - axis);
			for (let pair = 0; pair < 4; pair++) {
				const lower = (pair % mask) + Math.floor(pair / mask) * mask * 2;
				const upper = lower + mask;
				// Subtract opposing corners first so constant directions cancel exactly.
				realGradient[axis] +=
					(cornerValues[upper][0] - cornerValues[lower][0]) /
					(4 * fields.grid.spacing);
				imaginaryGradient[axis] +=
					(cornerValues[upper][1] - cornerValues[lower][1]) /
					(4 * fields.grid.spacing);
			}
			positions[sample * 3 + axis] = (base[axis] + 0.5) / dimensions[axis];
			vectors[sample * 3 + axis] =
				real * imaginaryGradient[axis] - imaginary * realGradient[axis];
		}
		peak = Math.max(
			peak,
			Math.hypot(...vectors.subarray(sample * 3, sample * 3 + 3)),
		);
	}

	if (peak === 0) return { vertices: new Float32Array(0), peak };

	// Half a sampling interval leaves room for the half-cell center offset.
	// Relative arrow lengths encode flux magnitude, never a time step.
	const lengthScale = 0.5 / Math.max(...samples) / peak;
	const vertices: number[] = [];
	for (let sample = 0; sample < count; sample++) {
		const delta = Array.from(
			vectors.subarray(sample * 3, sample * 3 + 3),
			(value) => value * lengthScale,
		);
		const length = Math.hypot(...delta);
		if (length === 0) continue;

		const center = positions.subarray(sample * 3, sample * 3 + 3);
		const head = delta.map((value, axis) => center[axis] + value / 2);
		const tail = delta.map((value, axis) => center[axis] - value / 2);
		// Cross with the least-aligned coordinate axis to form an arrowhead plane.
		const reference = delta
			.map(Math.abs)
			.indexOf(Math.min(...delta.map(Math.abs)));
		const side = [0, 0, 0];
		side[(reference + 1) % 3] = delta[(reference + 2) % 3];
		side[(reference + 2) % 3] = -delta[(reference + 1) % 3];
		const sideLength = Math.hypot(...side);
		const strength =
			Math.hypot(...vectors.subarray(sample * 3, sample * 3 + 3)) / peak;
		vertices.push(...tail, 0, strength, ...head, 1, strength);
		for (const sign of [-1, 1]) {
			const wing = delta.map(
				(value, axis) =>
					head[axis] -
					value / 4 +
					(((sign * side[axis]) / sideLength) * length) / 4,
			);
			vertices.push(...wing, 1, strength, ...head, 1, strength);
		}
	}
	return { vertices: new Float32Array(vertices), peak };
};

// Owns one snapshot of spatial current glyphs. Camera redraws do not advect it.
export class PhaseCurrent {
	visible = true;
	private vertexCount = 0;
	private vertexBuffer: GPUBuffer | null = null;
	private readonly pipeline: GPURenderPipeline;
	private readonly bindGroup: GPUBindGroup;

	constructor(
		private readonly gpu: FluidGPU,
		fieldsUniformBuffer: GPUBuffer,
	) {
		const layout = this.gpu.device.createBindGroupLayout({
			entries: [
				{
					binding: 0,
					visibility: GPUShaderStage.VERTEX,
					buffer: { type: "uniform" },
				},
			],
		});
		this.bindGroup = this.gpu.device.createBindGroup({
			layout,
			entries: [{ binding: 0, resource: { buffer: fieldsUniformBuffer } }],
		});
		this.pipeline = this.gpu.device.createRenderPipeline({
			layout: this.gpu.device.createPipelineLayout({
				bindGroupLayouts: [layout],
			}),
			vertex: {
				module: this.gpu.device.createShaderModule({
					code: currentShader,
				}),
				entryPoint: "vs_main",
				buffers: [
					{
						arrayStride: 20,
						attributes: [
							{ shaderLocation: 0, offset: 0, format: "float32x3" },
							{ shaderLocation: 1, offset: 12, format: "float32x2" },
						],
					},
				],
			},
			fragment: {
				module: this.gpu.device.createShaderModule({
					code: currentShader,
				}),
				entryPoint: "fs_main",
				targets: [
					{
						format: this.gpu.format,
						blend: {
							color: {
								srcFactor: "src-alpha",
								dstFactor: "one",
								operation: "add",
							},
							alpha: {
								srcFactor: "one",
								dstFactor: "one",
								operation: "add",
							},
						},
					},
				],
			},
			primitive: { topology: "line-list" },
			depthStencil: {
				format: "depth24plus",
				depthWriteEnabled: false,
				depthCompare: "less",
			},
		});
	}

	update(fields: FluidFields) {
		const { vertices, peak } = phaseCurrentGlyphs(fields);
		this.vertexCount = vertices.length / 5;
		this.vertexBuffer?.destroy();
		this.vertexBuffer = null;
		if (this.vertexCount === 0) return peak;

		this.vertexBuffer = createVertexBuffer(this.gpu.device, vertices);
		return peak;
	}

	encode(pass: GPURenderPassEncoder) {
		if (!this.visible || this.vertexBuffer === null) return;

		pass.setPipeline(this.pipeline);
		pass.setBindGroup(0, this.bindGroup);
		pass.setVertexBuffer(0, this.vertexBuffer);
		pass.draw(this.vertexCount);
	}

	dispose() {
		this.vertexBuffer?.destroy();
	}
}

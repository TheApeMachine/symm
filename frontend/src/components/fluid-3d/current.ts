import { currentShader } from "./field-shaders";
import { createVertexBuffer, type FluidGPU } from "./gpu";
import type { FluidParticleFrame } from "./wire";

/* Draw the solver's per-particle guidance velocity. Only arrow geometry and
relative display length are computed here; the wave is never differentiated. */
export const pilotCurrentGlyphs = (
	frame: FluidParticleFrame,
	spacing: number,
) => {
	const vectors = frame.pilotVel;
	if (vectors === null) return { vertices: new Float32Array(0), peak: null };

	if (
		vectors.length !== frame.count * 3 ||
		frame.pos.length !== frame.count * 3
	) {
		throw new Error(
			"pilot guidance and positions must match the particle count",
		);
	}
	const count = frame.count;
	const positions = frame.pos;
	let peak = 0;
	for (let particle = 0; particle < count; particle++) {
		peak = Math.max(
			peak,
			Math.hypot(...vectors.subarray(particle * 3, particle * 3 + 3)),
		);
	}
	if (peak === 0) return { vertices: new Float32Array(0), peak };

	// The longest direction marker spans two grid cells. This is a labelled
	// display scale, not integrated travel over a synthetic time interval.
	const lengthScale = (2 * spacing) / peak;
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
export class PilotCurrent {
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

	update(frame: FluidParticleFrame, spacing: number) {
		const { vertices, peak } = pilotCurrentGlyphs(frame, spacing);
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

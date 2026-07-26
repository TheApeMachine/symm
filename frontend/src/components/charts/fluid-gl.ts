/*
fluid-gl paints pilot-wave lattices from retained Float32 planes via WebGL2.
Dense grids stay off the JSON path; the GPU samples R32F textures directly.
*/

import type { FluidFieldLayer } from "#/collections/terminal";
import type { LatticePlane } from "#/providers/manifold-binary";
import {
	latestDisplay,
	latestLattice,
} from "#/providers/manifold-binary";

const DISPLAY_GAMMA = 0.55;

const VERT = `#version 300 es
in vec2 a_pos;
out vec2 v_uv;
void main() {
  v_uv = vec2(a_pos.x * 0.5 + 0.5, 1.0 - (a_pos.y * 0.5 + 0.5));
  gl_Position = vec4(a_pos, 0.0, 1.0);
}`;

const FRAG = `#version 300 es
precision highp float;
in vec2 v_uv;
out vec4 outColor;
uniform sampler2D u_primary;
uniform sampler2D u_gas;
uniform vec2 u_primaryExtent;
uniform vec2 u_gasExtent;
uniform float u_gamma;
uniform int u_layer;
uniform int u_contour;

float unitOf(sampler2D tex, vec2 extent) {
  float sample = texture(tex, v_uv).r;
  float span = max(extent.y - extent.x, 1e-6);
  float unit = clamp((sample - extent.x) / span, 0.0, 1.0);
  unit = unit > 0.0 ? pow(unit, u_gamma) : 0.0;
  if (u_contour == 1) {
    unit = floor(unit / 0.12) * 0.12;
  }
  return unit;
}

vec3 cmap(float t) {
  vec3 c0 = vec3(14.0, 12.0, 10.0);
  vec3 c1 = vec3(26.0, 34.0, 50.0);
  vec3 c2 = vec3(42.0, 106.0, 129.0);
  vec3 c3 = vec3(232.0, 163.0, 61.0);
  vec3 c4 = vec3(246.0, 214.0, 159.0);
  if (t < 0.4) return mix(c0, c1, t / 0.4);
  if (t < 0.6) return mix(c1, c2, (t - 0.4) / 0.2);
  if (t < 0.8) return mix(c2, c3, (t - 0.6) / 0.2);
  return mix(c3, c4, (t - 0.8) / 0.2);
}

void main() {
  float primary = unitOf(u_primary, u_primaryExtent);
  vec3 color = cmap(primary);
  if (u_layer == 2) {
    float gas = unitOf(u_gas, u_gasExtent);
    color = mix(color, vec3(48.0, 168.0, 196.0), gas * 0.38);
  }
  outColor = vec4(color / 255.0, 1.0);
}`;

type FluidGL = {
	gl: WebGL2RenderingContext;
	program: WebGLProgram;
	primary: WebGLTexture;
	gas: WebGLTexture;
	vao: WebGLVertexArrayObject;
	locations: {
		primary: WebGLUniformLocation;
		gas: WebGLUniformLocation;
		primaryExtent: WebGLUniformLocation;
		gasExtent: WebGLUniformLocation;
		gamma: WebGLUniformLocation;
		layer: WebGLUniformLocation;
		contour: WebGLUniformLocation;
	};
};

let renderer: FluidGL | null = null;

const compile = (
	gl: WebGL2RenderingContext,
	type: number,
	source: string,
): WebGLShader => {
	const shader = gl.createShader(type);

	if (shader === null) {
		throw new Error("webgl shader alloc failed");
	}

	gl.shaderSource(shader, source);
	gl.compileShader(shader);

	if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
		throw new Error(String(gl.getShaderInfoLog(shader)));
	}

	return shader;
};

const createTexture = (gl: WebGL2RenderingContext): WebGLTexture => {
	const texture = gl.createTexture();

	if (texture === null) {
		throw new Error("webgl texture alloc failed");
	}

	gl.bindTexture(gl.TEXTURE_2D, texture);
	gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
	gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
	gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
	gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
	return texture;
};

const ensure = (canvas: HTMLCanvasElement): FluidGL | null => {
	if (renderer !== null && renderer.gl.canvas === canvas) {
		return renderer;
	}

	const gl = canvas.getContext("webgl2", {
		alpha: false,
		antialias: false,
		preserveDrawingBuffer: false,
	});

	if (gl === null) {
		return null;
	}

	const program = gl.createProgram();

	if (program === null) {
		return null;
	}

	gl.attachShader(program, compile(gl, gl.VERTEX_SHADER, VERT));
	gl.attachShader(program, compile(gl, gl.FRAGMENT_SHADER, FRAG));
	gl.linkProgram(program);

	if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
		return null;
	}

	const buffer = gl.createBuffer();
	const vao = gl.createVertexArray();

	if (buffer === null || vao === null) {
		return null;
	}

	gl.bindVertexArray(vao);
	gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
	gl.bufferData(
		gl.ARRAY_BUFFER,
		new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]),
		gl.STATIC_DRAW,
	);
	const position = gl.getAttribLocation(program, "a_pos");
	gl.enableVertexAttribArray(position);
	gl.vertexAttribPointer(position, 2, gl.FLOAT, false, 0, 0);

	const primaryLoc = gl.getUniformLocation(program, "u_primary");
	const gasLoc = gl.getUniformLocation(program, "u_gas");
	const primaryExtent = gl.getUniformLocation(program, "u_primaryExtent");
	const gasExtent = gl.getUniformLocation(program, "u_gasExtent");
	const gamma = gl.getUniformLocation(program, "u_gamma");
	const layerLoc = gl.getUniformLocation(program, "u_layer");
	const contour = gl.getUniformLocation(program, "u_contour");

	if (
		primaryLoc === null ||
		gasLoc === null ||
		primaryExtent === null ||
		gasExtent === null ||
		gamma === null ||
		layerLoc === null ||
		contour === null
	) {
		return null;
	}

	renderer = {
		gl,
		program,
		primary: createTexture(gl),
		gas: createTexture(gl),
		vao,
		locations: {
			primary: primaryLoc,
			gas: gasLoc,
			primaryExtent,
			gasExtent,
			gamma,
			layer: layerLoc,
			contour,
		},
	};
	return renderer;
};

const upload = (
	gl: WebGL2RenderingContext,
	texture: WebGLTexture,
	plane: LatticePlane,
) => {
	gl.bindTexture(gl.TEXTURE_2D, texture);
	gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
	gl.texImage2D(
		gl.TEXTURE_2D,
		0,
		gl.R32F,
		plane.width,
		plane.height,
		0,
		gl.RED,
		gl.FLOAT,
		plane.samples,
	);
};

const layerCode = (layer: FluidFieldLayer): number => {
	if (layer === "Gas") {
		return 0;
	}

	if (layer === "Coherence") {
		return 1;
	}

	return 2;
};

const primaryPlane = (layer: FluidFieldLayer): LatticePlane | null => {
	if (layer === "Gas") {
		return latestLattice("rho");
	}

	return latestLattice("psi");
};

/*
drawFluidDisplay blits the backend-composited RGBA texture. Returns false when
no display frame is retained so callers can fall back to plane shading.
*/
export const drawFluidDisplay = (
	canvas: HTMLCanvasElement,
	width: number,
	height: number,
): boolean => {
	const frame = latestDisplay();

	if (frame === null || width <= 0 || height <= 0) {
		return false;
	}

	const context = canvas.getContext("2d");

	if (context === null) {
		return false;
	}

	const dpr = Math.max(1, window.devicePixelRatio || 1);
	const pixelWidth = Math.max(1, Math.floor(width * dpr));
	const pixelHeight = Math.max(1, Math.floor(height * dpr));

	if (canvas.width !== pixelWidth || canvas.height !== pixelHeight) {
		canvas.width = pixelWidth;
		canvas.height = pixelHeight;
	}

	const tile = document.createElement("canvas");
	tile.width = frame.width;
	tile.height = frame.height;
	const tileContext = tile.getContext("2d");

	if (tileContext === null) {
		return false;
	}

	const image = tileContext.createImageData(frame.width, frame.height);
	image.data.set(frame.rgba);
	tileContext.putImageData(image, 0, 0);
	context.imageSmoothingEnabled = true;
	context.imageSmoothingQuality = "high";
	context.clearRect(0, 0, pixelWidth, pixelHeight);
	context.drawImage(tile, 0, 0, pixelWidth, pixelHeight);
	return true;
};

/*
drawFluidFieldGL uploads retained binary lattices and draws the pilot-wave field.
Returns false when WebGL2 is unavailable so the caller can fall back to Canvas2D.
*/
export const drawFluidFieldGL = (
	canvas: HTMLCanvasElement,
	width: number,
	height: number,
	layer: FluidFieldLayer,
	contour: boolean,
): boolean => {
	if (drawFluidDisplay(canvas, width, height)) {
		return true;
	}

	const primary = primaryPlane(layer);
	const gas = latestLattice("rho");

	if (primary === null) {
		return false;
	}

	const state = ensure(canvas);

	if (state === null) {
		return false;
	}

	const { gl } = state;
	const dpr = Math.max(1, window.devicePixelRatio || 1);
	const pixelWidth = Math.max(1, Math.floor(width * dpr));
	const pixelHeight = Math.max(1, Math.floor(height * dpr));

	if (canvas.width !== pixelWidth || canvas.height !== pixelHeight) {
		canvas.width = pixelWidth;
		canvas.height = pixelHeight;
	}

	upload(gl, state.primary, primary);
	upload(gl, state.gas, gas ?? primary);

	gl.viewport(0, 0, pixelWidth, pixelHeight);
	// biome-ignore lint: biome's use* hooks false positive
	gl.useProgram(state.program);
	gl.bindVertexArray(state.vao);

	gl.activeTexture(gl.TEXTURE0);
	gl.bindTexture(gl.TEXTURE_2D, state.primary);
	gl.uniform1i(state.locations.primary, 0);

	gl.activeTexture(gl.TEXTURE1);
	gl.bindTexture(gl.TEXTURE_2D, state.gas);
	gl.uniform1i(state.locations.gas, 1);

	gl.uniform2f(state.locations.primaryExtent, primary.min, primary.max);
	gl.uniform2f(
		state.locations.gasExtent,
		(gas ?? primary).min,
		(gas ?? primary).max,
	);
	gl.uniform1f(state.locations.gamma, DISPLAY_GAMMA);
	gl.uniform1i(state.locations.layer, layerCode(layer));
	gl.uniform1i(state.locations.contour, contour ? 1 : 0);

	gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
	return true;
};

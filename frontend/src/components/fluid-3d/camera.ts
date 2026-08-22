const TAU = Math.PI * 2;
const POLAR_LIMIT = 0.05;
const ROTATE_SPEED = 0.005;
const ZOOM_SPEED = 0.001;
const MINIMUM_RADIUS = 0.15;
const MAXIMUM_RADIUS = 8;

export type Ray = {
	origin: [number, number, number];
	direction: [number, number, number];
};

const subtract = (
	left: [number, number, number],
	right: [number, number, number],
): [number, number, number] => [
	left[0] - right[0],
	left[1] - right[1],
	left[2] - right[2],
];

const cross = (
	left: [number, number, number],
	right: [number, number, number],
): [number, number, number] => [
	left[1] * right[2] - left[2] * right[1],
	left[2] * right[0] - left[0] * right[2],
	left[0] * right[1] - left[1] * right[0],
];

const dot = (
	left: [number, number, number],
	right: [number, number, number],
) => left[0] * right[0] + left[1] * right[1] + left[2] * right[2];

const normalize = (
	vector: [number, number, number],
): [number, number, number] => {
	const length = Math.hypot(vector[0], vector[1], vector[2]);
	return [vector[0] / length, vector[1] / length, vector[2] / length];
};

export const multiply4 = (left: Float32Array, right: Float32Array) => {
	const product = new Float32Array(16);

	for (let column = 0; column < 4; column += 1) {
		for (let row = 0; row < 4; row += 1) {
			product[column * 4 + row] =
				left[row] * right[column * 4] +
				left[row + 4] * right[column * 4 + 1] +
				left[row + 8] * right[column * 4 + 2] +
				left[row + 12] * right[column * 4 + 3];
		}
	}

	return product;
};

export const invert4 = (matrix: Float32Array) => {
	const inverse = new Float32Array(16);
	const m00 = matrix[0]!;
	const m01 = matrix[1]!;
	const m02 = matrix[2]!;
	const m03 = matrix[3]!;
	const m10 = matrix[4]!;
	const m11 = matrix[5]!;
	const m12 = matrix[6]!;
	const m13 = matrix[7]!;
	const m20 = matrix[8]!;
	const m21 = matrix[9]!;
	const m22 = matrix[10]!;
	const m23 = matrix[11]!;
	const m30 = matrix[12]!;
	const m31 = matrix[13]!;
	const m32 = matrix[14]!;
	const m33 = matrix[15]!;
	inverse[0] =
		m11 * m22 * m33 -
		m11 * m23 * m32 -
		m21 * m12 * m33 +
		m21 * m13 * m32 +
		m31 * m12 * m23 -
		m31 * m13 * m22;
	inverse[4] =
		-m10 * m22 * m33 +
		m10 * m23 * m32 +
		m20 * m12 * m33 -
		m20 * m13 * m32 -
		m30 * m12 * m23 +
		m30 * m13 * m22;
	inverse[8] =
		m10 * m21 * m33 -
		m10 * m23 * m31 -
		m20 * m11 * m33 +
		m20 * m13 * m31 +
		m30 * m11 * m23 -
		m30 * m13 * m21;
	inverse[12] =
		-m10 * m21 * m32 +
		m10 * m22 * m31 +
		m20 * m11 * m32 -
		m20 * m12 * m31 -
		m30 * m11 * m22 +
		m30 * m12 * m21;
	inverse[1] =
		-m01 * m22 * m33 +
		m01 * m23 * m32 +
		m21 * m02 * m33 -
		m21 * m03 * m32 -
		m31 * m02 * m23 +
		m31 * m03 * m22;
	inverse[5] =
		m00 * m22 * m33 -
		m00 * m23 * m32 -
		m20 * m02 * m33 +
		m20 * m03 * m32 +
		m30 * m02 * m23 -
		m30 * m03 * m22;
	inverse[9] =
		-m00 * m21 * m33 +
		m00 * m23 * m31 +
		m20 * m01 * m33 -
		m20 * m03 * m31 -
		m30 * m01 * m23 +
		m30 * m03 * m21;
	inverse[13] =
		m00 * m21 * m32 -
		m00 * m22 * m31 -
		m20 * m01 * m32 +
		m20 * m02 * m31 +
		m30 * m01 * m22 -
		m30 * m02 * m21;
	inverse[2] =
		m01 * m12 * m33 -
		m01 * m13 * m32 -
		m11 * m02 * m33 +
		m11 * m03 * m32 +
		m31 * m02 * m13 -
		m31 * m03 * m12;
	inverse[6] =
		-m00 * m12 * m33 +
		m00 * m13 * m32 +
		m10 * m02 * m33 -
		m10 * m03 * m32 -
		m30 * m02 * m13 +
		m30 * m03 * m12;
	inverse[10] =
		m00 * m11 * m33 -
		m00 * m13 * m31 -
		m10 * m01 * m33 +
		m10 * m03 * m31 +
		m30 * m01 * m13 -
		m30 * m03 * m11;
	inverse[14] =
		-m00 * m11 * m32 +
		m00 * m12 * m31 +
		m10 * m01 * m32 -
		m10 * m02 * m31 -
		m30 * m01 * m12 +
		m30 * m02 * m11;
	inverse[3] =
		-m01 * m12 * m23 +
		m01 * m13 * m22 +
		m11 * m02 * m23 -
		m11 * m03 * m22 -
		m21 * m02 * m13 +
		m21 * m03 * m12;
	inverse[7] =
		m00 * m12 * m23 -
		m00 * m13 * m22 -
		m10 * m02 * m23 +
		m10 * m03 * m22 +
		m20 * m02 * m13 -
		m20 * m03 * m12;
	inverse[11] =
		-m00 * m11 * m23 +
		m00 * m13 * m21 +
		m10 * m01 * m23 -
		m10 * m03 * m21 -
		m20 * m01 * m13 +
		m20 * m03 * m11;
	inverse[15] =
		m00 * m11 * m22 -
		m00 * m12 * m21 -
		m10 * m01 * m22 +
		m10 * m02 * m21 +
		m20 * m01 * m12 -
		m20 * m02 * m11;
	const determinant = m00 * inverse[0] + m01 * inverse[4] + m02 * inverse[8] + m03 * inverse[12];
	const scale = 1 / determinant;

	for (let index = 0; index < 16; index += 1) {
		inverse[index]! *= scale;
	}

	return inverse;
};

const perspective = (fovY: number, aspect: number, near: number, far: number) => {
	const f = 1 / Math.tan(fovY / 2);
	const matrix = new Float32Array(16);
	matrix[0] = f / aspect;
	matrix[5] = f;
	matrix[10] = far / (near - far);
	matrix[11] = -1;
	matrix[14] = (near * far) / (near - far);
	return matrix;
};

const lookAt = (
	eye: [number, number, number],
	target: [number, number, number],
	up: [number, number, number],
) => {
	const axisZ = normalize(subtract(eye, target));
	const axisX = normalize(cross(up, axisZ));
	const axisY = cross(axisZ, axisX);
	const matrix = new Float32Array(16);
	matrix[0] = axisX[0];
	matrix[1] = axisY[0];
	matrix[2] = axisZ[0];
	matrix[4] = axisX[1];
	matrix[5] = axisY[1];
	matrix[6] = axisZ[1];
	matrix[8] = axisX[2];
	matrix[9] = axisY[2];
	matrix[10] = axisZ[2];
	matrix[12] = -dot(axisX, eye);
	matrix[13] = -dot(axisY, eye);
	matrix[14] = -dot(axisZ, eye);
	matrix[15] = 1;
	return matrix;
};

const transform4 = (matrix: Float32Array, vector: [number, number, number, number]) => [
	matrix[0]! * vector[0] +
		matrix[4]! * vector[1] +
		matrix[8]! * vector[2] +
		matrix[12]! * vector[3],
	matrix[1]! * vector[0] +
		matrix[5]! * vector[1] +
		matrix[9]! * vector[2] +
		matrix[13]! * vector[3],
	matrix[2]! * vector[0] +
		matrix[6]! * vector[1] +
		matrix[10]! * vector[2] +
		matrix[14]! * vector[3],
	matrix[3]! * vector[0] +
		matrix[7]! * vector[1] +
		matrix[11]! * vector[2] +
		matrix[15]! * vector[3],
];

/*
OrbitCamera is a Y-up orbit around the unit-box centre. It owns the view and
projection matrices the WebGPU inspector submits each frame.
*/
export class OrbitCamera {
	readonly target: [number, number, number] = [0.5, 0.5, 0.5];
	position: [number, number, number] = [1.65, 1.35, 1.65];
	right: [number, number, number] = [1, 0, 0];
	up: [number, number, number] = [0, 1, 0];
	viewProj = new Float32Array(16);
	invViewProj = new Float32Array(16);
	private azimuth = 0;
	private polar = Math.PI / 4;
	private radius = 1.8;
	private aspect = 1;
	private readonly fovY = (48 * Math.PI) / 180;
	private readonly near = 0.001;
	private readonly far = 20;
	private element: HTMLElement | null = null;
	private dragging = false;
	private onChange: (() => void) | null = null;

	constructor() {
		this.lookFrom(1.65, 1.35, 1.65);
		this.recompute();
	}

	attach(element: HTMLElement, onChange: () => void) {
		this.element = element;
		this.onChange = onChange;
		element.addEventListener("pointerdown", this.onPointerDown);
		element.addEventListener("pointermove", this.onPointerMove);
		element.addEventListener("pointerup", this.onPointerUp);
		element.addEventListener("pointercancel", this.onPointerUp);
		element.addEventListener("wheel", this.onWheel, { passive: false });
	}

	detach() {
		const element = this.element;

		if (element === null) {
			return;
		}

		element.removeEventListener("pointerdown", this.onPointerDown);
		element.removeEventListener("pointermove", this.onPointerMove);
		element.removeEventListener("pointerup", this.onPointerUp);
		element.removeEventListener("pointercancel", this.onPointerUp);
		element.removeEventListener("wheel", this.onWheel);
		this.element = null;
		this.onChange = null;
	}

	setAspect(aspect: number) {
		this.aspect = aspect;
		this.recompute();
	}

	ray(ndcX: number, ndcY: number): Ray {
		const nearPoint = this.unproject(ndcX, ndcY, 0);
		const farPoint = this.unproject(ndcX, ndcY, 1);
		return {
			origin: this.position,
			direction: normalize(subtract(farPoint, nearPoint)),
		};
	}

	private lookFrom(x: number, y: number, z: number) {
		const dx = x - this.target[0];
		const dy = y - this.target[1];
		const dz = z - this.target[2];
		this.radius = Math.hypot(dx, dy, dz);
		this.azimuth = Math.atan2(dx, dz);
		this.polar = Math.acos(dy / this.radius);
	}

	private recompute() {
		const sinPolar = Math.sin(this.polar);
		this.position = [
			this.target[0] + this.radius * sinPolar * Math.sin(this.azimuth),
			this.target[1] + this.radius * Math.cos(this.polar),
			this.target[2] + this.radius * sinPolar * Math.cos(this.azimuth),
		];
		const view = lookAt(this.position, this.target, [0, 1, 0]);
		this.right = [view[0]!, view[4]!, view[8]!];
		this.up = [view[1]!, view[5]!, view[9]!];
		const projection = perspective(this.fovY, this.aspect, this.near, this.far);
		this.viewProj = multiply4(projection, view);
		this.invViewProj = invert4(this.viewProj);
	}

	private unproject(ndcX: number, ndcY: number, ndcZ: number): [number, number, number] {
		const clip: [number, number, number, number] = [ndcX, ndcY, ndcZ, 1];
		const world = transform4(this.invViewProj, clip);
		return [world[0] / world[3], world[1] / world[3], world[2] / world[3]];
	}

	private readonly onPointerDown = (event: PointerEvent) => {
		if (event.button !== 0) {
			return;
		}

		this.dragging = true;
		this.element?.setPointerCapture(event.pointerId);
	};

	private readonly onPointerMove = (event: PointerEvent) => {
		if (!this.dragging) {
			return;
		}

		this.azimuth -= event.movementX * ROTATE_SPEED;
		this.azimuth = ((this.azimuth % TAU) + TAU) % TAU;
		this.polar = Math.min(
			Math.PI - POLAR_LIMIT,
			Math.max(POLAR_LIMIT, this.polar - event.movementY * ROTATE_SPEED),
		);
		this.recompute();
		this.onChange?.();
	};

	private readonly onPointerUp = (event: PointerEvent) => {
		this.dragging = false;

		if (this.element?.hasPointerCapture(event.pointerId) === true) {
			this.element.releasePointerCapture(event.pointerId);
		}
	};

	private readonly onWheel = (event: WheelEvent) => {
		event.preventDefault();
		this.radius = Math.min(
			MAXIMUM_RADIUS,
			Math.max(MINIMUM_RADIUS, this.radius * Math.exp(event.deltaY * ZOOM_SPEED)),
		);
		this.recompute();
		this.onChange?.();
	};
}

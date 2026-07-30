import type { RefObject } from "react";
import * as THREE from "three";
import type { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import type { Simulator } from "./simulator";

const CAMERA_STORAGE_KEY = "caramba.nodeGraphLegacy.camera";

interface CameraProps {
	cameraRef: RefObject<THREE.PerspectiveCamera | null>;
	controlsRef: RefObject<OrbitControls | null>;
	simulatorRef: RefObject<Simulator | null>;
	lastLayoutBoundsRef: RefObject<{
		center: THREE.Vector3;
		radius: number;
	} | null>;
	lastCameraSaveMsRef: RefObject<number>;
}

export const Camera = ({
	cameraRef,
	controlsRef,
	simulatorRef,
	lastLayoutBoundsRef,
	lastCameraSaveMsRef,
}: CameraProps) => {
	const fitCameraToGraph = () => {
		if (!cameraRef.current || !controlsRef.current || !simulatorRef.current)
			return;
		const positionTexture = simulatorRef.current.getPositionTexture();
		if (!positionTexture) return;

		const bounds = lastLayoutBoundsRef.current;
		const center = bounds?.center ?? new THREE.Vector3(0, 0, 0);
		const radius = bounds?.radius ?? 800;

		// Preserve current view direction relative to OrbitControls target.
		const target = controlsRef.current.target.clone();
		const dir = cameraRef.current.position.clone().sub(target);
		if (dir.lengthSq() < 1e-6) dir.set(0, 0, 1);
		dir.normalize();

		const distance = Math.max(200, radius * 2.2);
		cameraRef.current.position.copy(
			center.clone().add(dir.multiplyScalar(distance)),
		);
		cameraRef.current.lookAt(center);
		cameraRef.current.fov = 50;
		cameraRef.current.updateProjectionMatrix();
		controlsRef.current.target.copy(center);
		controlsRef.current.update();
	};

	const setOrbitTargetToLayoutCenter = () => {
		if (!controlsRef.current) return;
		const bounds = lastLayoutBoundsRef.current;
		if (!bounds) return;
		controlsRef.current.target.copy(bounds.center);
		controlsRef.current.update();
	};

	const restoreCameraState = () => {
		if (!cameraRef.current || !controlsRef.current) return;
		if (typeof window === "undefined") return;
		const raw = window.localStorage.getItem(CAMERA_STORAGE_KEY);
		if (!raw) return;
		let parsed: unknown;
		try {
			parsed = JSON.parse(raw);
		} catch {
			return;
		}
		if (!parsed || typeof parsed !== "object") return;
		const p = parsed as {
			pos?: { x: number; y: number; z: number };
			target?: { x: number; y: number; z: number };
			fov?: number;
		};
		if (p.pos) cameraRef.current.position.set(p.pos.x, p.pos.y, p.pos.z);
		if (typeof p.fov === "number") {
			cameraRef.current.fov = p.fov;
			cameraRef.current.updateProjectionMatrix();
		}
		if (p.target)
			controlsRef.current.target.set(p.target.x, p.target.y, p.target.z);
		controlsRef.current.update();
	};

	const saveCameraState = () => {
		if (!cameraRef.current || !controlsRef.current) return;
		if (typeof window === "undefined") return;
		const now = performance.now();
		// Cheap throttle to avoid spamming localStorage while dragging
		if (now - lastCameraSaveMsRef.current < 150) return;
		lastCameraSaveMsRef.current = now;

		const pos = cameraRef.current.position;
		const target = controlsRef.current.target;
		const payload = {
			fov: cameraRef.current.fov,
			pos: { x: pos.x, y: pos.y, z: pos.z },
			target: { x: target.x, y: target.y, z: target.z },
		};
		window.localStorage.setItem(CAMERA_STORAGE_KEY, JSON.stringify(payload));
	};

	return {
		fitCameraToGraph,
		setOrbitTargetToLayoutCenter,
		restoreCameraState,
		saveCameraState,
	};
};

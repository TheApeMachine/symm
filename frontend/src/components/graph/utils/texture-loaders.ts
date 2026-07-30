import * as THREE from "three";

/**
 * Load the node texture (original uses textures/new_circle.png)
 */
export async function loadNodeTexture(): Promise<THREE.Texture> {
	return new Promise((resolve) => {
		const loader = new THREE.TextureLoader();
		loader.load(
			"/textures/new_circle.png",
			(texture) => {
				texture.minFilter = THREE.LinearFilter;
				texture.magFilter = THREE.LinearFilter;
				texture.needsUpdate = true;
				resolve(texture);
			},
			undefined,
			(error) => {
				console.warn(
					"Failed to load node texture, falling back to canvas",
					error,
				);
				// Fallback to generated texture if file missing
				const canvas = document.createElement("canvas");
				canvas.width = 64;
				canvas.height = 64;
				const ctx = canvas.getContext("2d");
				if (!ctx) {
					resolve(new THREE.Texture());
					return;
				}
				const gradient = ctx.createRadialGradient(32, 32, 0, 32, 32, 32);
				gradient.addColorStop(0, "rgba(255, 255, 255, 1)");
				gradient.addColorStop(0.5, "rgba(255, 255, 255, 0.5)");
				gradient.addColorStop(1, "rgba(255, 255, 255, 0)");
				ctx.fillStyle = gradient;
				ctx.fillRect(0, 0, 64, 64);
				resolve(new THREE.CanvasTexture(canvas));
			},
		);
	});
}

/**
 * Load the threat texture (original uses textures/crosshair.png)
 */
export async function loadThreatTexture(): Promise<THREE.Texture> {
	return new Promise((resolve) => {
		const loader = new THREE.TextureLoader();
		loader.load(
			"/textures/crosshair.png",
			(texture) => {
				texture.minFilter = THREE.LinearFilter;
				texture.magFilter = THREE.LinearFilter;
				texture.needsUpdate = true;
				resolve(texture);
			},
			undefined,
			(error) => {
				console.warn("Failed to load threat texture", error);
				resolve(new THREE.Texture()); // Return empty texture on fail
			},
		);
	});
}

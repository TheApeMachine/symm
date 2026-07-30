import * as THREE from "three";

export const generateLayeredLayoutTexture =
	(
		parseLayerToken: (name: string) => {
			layer: number;
			token: number;
		} | null,
		setLayoutBoundsFromMinMax: (
			min: { x: number; y: number; z: number },
			max: { x: number; y: number; z: number },
		) => void,
	) =>
	(nodeNames: string[], nodesWidth: number): THREE.DataTexture | null => {
		const parsed = nodeNames.map((n) => (n ? parseLayerToken(n) : null));
		const valid = parsed.filter(
			(x): x is { layer: number; token: number } => x !== null,
		);
		if (valid.length === 0) return null;

		let maxLayer = 0;
		let maxToken = 0;
		for (const { layer, token } of valid) {
			if (layer > maxLayer) maxLayer = layer;
			if (token > maxToken) maxToken = token;
		}

		const layerCount = maxLayer + 1;
		const tokenCount = maxToken + 1;

		const layerSpacing = 400;
		const tokenSpacing = 80;
		const x0 = -((layerCount - 1) * layerSpacing) / 2;
		const y0 = -((tokenCount - 1) * tokenSpacing) / 2;
		const z0 = -((layerCount - 1) * 10) / 2;

		const textureArray = new Float32Array(nodesWidth * nodesWidth * 4);
		const min = {
			x: Number.POSITIVE_INFINITY,
			y: Number.POSITIVE_INFINITY,
			z: Number.POSITIVE_INFINITY,
		};
		const max = {
			x: Number.NEGATIVE_INFINITY,
			y: Number.NEGATIVE_INFINITY,
			z: Number.NEGATIVE_INFINITY,
		};
		for (let i = 0; i < textureArray.length; i += 4) {
			textureArray[i] = -1.0;
			textureArray[i + 1] = -1.0;
			textureArray[i + 2] = -1.0;
			textureArray[i + 3] = -1.0;
		}

		for (let nodeIndex = 0; nodeIndex < nodeNames.length; nodeIndex++) {
			const p = parsed[nodeIndex];
			if (!p) continue;
			const x = x0 + p.layer * layerSpacing;
			const y = y0 + p.token * tokenSpacing;
			const z = z0 + p.layer * 10;

			const base = nodeIndex * 4;
			textureArray[base] = x;
			textureArray[base + 1] = y;
			textureArray[base + 2] = z;
			textureArray[base + 3] = 1.0;

			if (x < min.x) min.x = x;
			if (y < min.y) min.y = y;
			if (z < min.z) min.z = z;
			if (x > max.x) max.x = x;
			if (y > max.y) max.y = y;
			if (z > max.z) max.z = z;
		}

		setLayoutBoundsFromMinMax(min, max);
		const texture = new THREE.DataTexture(
			textureArray,
			nodesWidth,
			nodesWidth,
			THREE.RGBAFormat,
			THREE.FloatType,
		);
		texture.needsUpdate = true;
		return texture;
	};

import fs from "node:fs/promises";
import path from "node:path";

import esbuild from "esbuild";
import type { Plugin } from "vite";

const capnpArtifactPattern = /\.capnp\.js(?:\?.*)?$/;

/*
bundleCapnpArtifact inlines capnpc-ts CommonJS into browser ESM.
Transform-only esbuild leaves runtime require() calls that fail in the browser.
*/
const bundleCapnpArtifact = async (filePath: string) => {
	const code = await fs.readFile(filePath, "utf8");

	const result = await esbuild.build({
		stdin: {
			contents: code,
			loader: "js",
			resolveDir: path.dirname(filePath),
			sourcefile: filePath,
		},
		bundle: true,
		format: "esm",
		platform: "browser",
		sourcemap: true,
		write: false,
	});

	const output = result.outputFiles[0].text;
	const exportMatch = output.match(/export default (\w+)\(\);/);

	if (exportMatch === null) {
		return output;
	}

	const moduleInitializer = exportMatch[1];

	return output.replace(
		exportMatch[0],
		`const __capnpModule = ${moduleInitializer}();
export default __capnpModule;
export const Artifact = __capnpModule.Artifact;`,
	);
};

/*
capnpCjsToEsm serves bundled capnp artifacts through Vite's load hook.
*/
export const capnpCjsToEsm = (): Plugin => ({
	name: "capnp-cjs-to-esm",
	enforce: "pre",
	async load(id) {
		if (!capnpArtifactPattern.test(id)) {
			return null;
		}

		const filePath = id.split("?")[0];

		return bundleCapnpArtifact(filePath);
	},
});

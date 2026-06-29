import tailwindcss from "@tailwindcss/vite";
import { devtools } from "@tanstack/devtools-vite";

import { tanstackStart } from "@tanstack/react-start/plugin/vite";

import viteReact from "@vitejs/plugin-react";
import { defineConfig } from "vite";

import { capnpCjsToEsm } from "./vite.capnp-cjs-to-esm";

const config = defineConfig({
	resolve: {
		tsconfigPaths: true,
	},
	optimizeDeps: {
		include: ["capnp-ts"],
		needsInterop: ["capnp-ts"],
		// capnp-ts ships compiled JS beside TS sources; prefer JS so rolldown does not bind type-only TS exports.
		rolldownOptions: {
			resolve: {
				extensions: [
					".js",
					".mjs",
					".cjs",
					".jsx",
					".ts",
					".tsx",
					".mts",
					".css",
					".json",
				],
			},
		},
	},
	assetsInclude: ["**/*.wasm"],
	plugins: [
		capnpCjsToEsm(),
		devtools(),
		tailwindcss(),
		tanstackStart(),
		viteReact(),
	],
});

export default config;

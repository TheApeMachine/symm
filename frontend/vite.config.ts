import tailwindcss from "@tailwindcss/vite";
import { devtools } from "@tanstack/devtools-vite";

import { tanstackStart } from "@tanstack/react-start/plugin/vite";

import viteReact from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";
import path from "path";
import { nodePolyfills } from 'vite-plugin-node-polyfills';

const config = defineConfig({
	resolve: {
		alias: {
			"node:module": path.resolve(__dirname, "./src/empty.ts"),
			"node:net": path.resolve(__dirname, "./src/empty.ts"),
			"node:events": path.resolve(__dirname, "./src/empty.ts"),
			"node:crypto": path.resolve(__dirname, "./src/empty.ts"),
			"crypto": path.resolve(__dirname, "./src/empty.ts"),
			"events": path.resolve(__dirname, "./src/empty.ts"),
			"net": path.resolve(__dirname, "./src/empty.ts"),
			"tls": path.resolve(__dirname, "./src/empty.ts"),
			"stream": path.resolve(__dirname, "./src/empty.ts"),
			"url": path.resolve(__dirname, "./src/empty.ts"),
			"http": path.resolve(__dirname, "./src/empty.ts"),
			"https": path.resolve(__dirname, "./src/empty.ts"),
		},
		tsconfigPaths: true,
	},
	assetsInclude: ["**/*.wasm"],
	/*
		Perspective's engine and viewer are WebAssembly modules built against
		modern language features; anything below esnext fails to parse them.
	*/
	build: {
		target: "esnext",
	},
	plugins: [
		nodePolyfills({
			globals: { Buffer: true },
			exclude: [
				'module',
				'net',
				'events',
				'crypto',
				'tls',
				'stream',
				'url',
				'http',
				'https',
			],
		}),
		devtools(), 
		tailwindcss(), 
		tanstackStart(), 
		viteReact()
	],
	test: {
		/*
			Runs before every test file, in whichever environment that file
			declared. It fills gaps the environment leaves rather than
			configuring one — see the file for what and why.
		*/
		setupFiles: ["./vitest.setup.ts"],
	},
	server: {
		watch: {
			ignored: ["**/src-tauri/**"],
		},
	},
});

export default config;

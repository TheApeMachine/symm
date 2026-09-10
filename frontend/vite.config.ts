import tailwindcss from "@tailwindcss/vite";
import { devtools } from "@tanstack/devtools-vite";

import { tanstackStart } from "@tanstack/react-start/plugin/vite";

import viteReact from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

const config = defineConfig({
	resolve: {
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
	plugins: [devtools(), tailwindcss(), tanstackStart(), viteReact()],
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

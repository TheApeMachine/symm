import tailwindcss from "@tailwindcss/vite";
import { devtools } from "@tanstack/devtools-vite";

import { tanstackStart } from "@tanstack/react-start/plugin/vite";

import viteReact from "@vitejs/plugin-react";
import { defineConfig } from "vite";

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
	server: {
		watch: {
			ignored: ["**/src-tauri/**"],
		},
	},
});

export default config;

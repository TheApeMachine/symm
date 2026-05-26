import { defineConfig } from "vite";
import { devtools } from "@tanstack/devtools-vite";

import { tanstackStart } from "@tanstack/react-start/plugin/vite";

import viteReact from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const config = defineConfig({
	resolve: { tsconfigPaths: true },
	assetsInclude: ["**/*.wasm"],
	server: {
		proxy: {
			"/ws": {
				target: "http://127.0.0.1:8765",
				ws: true,
				changeOrigin: true,
			},
		},
	},
	plugins: [devtools(), tailwindcss(), tanstackStart(), viteReact()],
});

export default config;

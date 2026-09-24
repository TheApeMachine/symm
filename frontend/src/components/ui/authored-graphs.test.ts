import type React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { compileUI, type FlumeGraph } from "./compiler";
import { renderUIRoute } from "./renderer";

/*
The graphs that ship must draw something.

A surface authored as nodes has no compiler of its own until it is rendered, so
a component that does not exist, a prop that is not on it, or a select given a
value outside its options would all reach a browser as a blank panel. Compiling
every shipped graph here is what turns those into a failing test instead.

The graphs are discovered rather than listed, so a surface authored tomorrow is
covered by this the moment it exists.
*/
const authored = import.meta.glob("../../../../manifest/ui_*.json", {
	eager: true,
}) as Record<string, { default: FlumeGraph }>;

describe("the UI graphs that ship", () => {
	it("finds them", () => {
		expect(Object.keys(authored).length).toBeGreaterThan(0);
	});

	for (const [file, module] of Object.entries(authored)) {
		const name =
			file
				.split("/")
				.pop()
				?.replace(/\.json$/, "") ?? file;

		describe(name, () => {
			const compiled = compileUI(module.default);

			it("compiles without diagnostics", () => {
				expect(compiled.diagnostics).toEqual([]);
			});

			it("declares the path it draws", () => {
				expect(compiled.routes).toHaveLength(1);
				expect(compiled.routes[0].path).toMatch(/^\//);
			});

			it("renders through the real components", () => {
				// Compiling only proves the graph names things that exist.
				// Rendering is what proves those things accept what the graph
				// passes them, which is where an authored surface actually
				// breaks.
				const markup = renderToStaticMarkup(
					renderUIRoute(compiled.routes[0]) as React.ReactElement,
				);

				expect(markup).toContain(
					`data-route-path="${compiled.routes[0].path}"`,
				);

				// Every word the graph declares has to survive into the
				// markup. A component that silently ignored what it was
				// passed would still render, and still be wrong.
				//
				// Words a node only shows under a choice are left out, since
				// a pane behind an unselected tab is deliberately not drawn.
				const words: string[] = [];

				const collect = (nodes: (typeof compiled.routes)[0]["components"]) => {
					for (const node of nodes) {
						if (node.visibleWhen) {
							continue;
						}

						for (const key of ["title", "value", "label"]) {
							const word = node.props?.[key];

							if (typeof word === "string" && word.length > 0) {
								words.push(word);
							}
						}

						collect(node.children ?? []);
					}
				};

				collect(compiled.routes[0].components);
				expect(words.length).toBeGreaterThan(0);

				for (const word of words) {
					expect(markup).toContain(word);
				}
			});

			it("wires every choice it declares", () => {
				// A surface that declares state nobody selects, or reveals a
				// pane under a choice nothing can make, is a dead control.
				// Both halves have to be present for the state to mean
				// anything.
				// Tracked as key/value pairs, not just keys: a choice that can
				// be made but reveals nothing is a control that does nothing,
				// and counting keys alone would let three working tabs cover
				// for a fourth that is wired to no pane.
				const selected = new Set<string>();
				const revealed = new Set<string>();
				const pair = (binding: { key: string; value: unknown }) =>
					`${binding.key}=${String(binding.value)}`;

				const walk = (nodes: (typeof compiled.routes)[0]["components"]) => {
					for (const node of nodes) {
						if (node.selects) {
							selected.add(pair(node.selects));
						}

						if (node.visibleWhen) {
							revealed.add(pair(node.visibleWhen));
						}

						walk(node.children ?? []);
					}
				};

				walk(compiled.routes[0].components);

				const declared = compiled.routes[0].state ?? {};

				for (const [key, value] of Object.entries(declared)) {
					// The choice the surface opens on has to draw something.
					expect(revealed).toContain(`${key}=${String(value)}`);
				}

				for (const choice of selected) {
					expect(revealed).toContain(choice);
					expect(declared).toHaveProperty(choice.split("=")[0]);
				}

				for (const choice of revealed) {
					expect(declared).toHaveProperty(choice.split("=")[0]);
				}
			});

			it("draws a component tree, not an empty route", () => {
				const count = (
					nodes: (typeof compiled.routes)[0]["components"],
				): number =>
					nodes.reduce(
						(total, node) => total + 1 + count(node.children ?? []),
						0,
					);

				expect(count(compiled.routes[0].components)).toBeGreaterThan(1);
			});
		});
	}
});

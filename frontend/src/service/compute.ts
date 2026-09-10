import { useEffect, useState } from "react";
import { hubBaseUrl } from "#/lib/hub";

/*
The pipeline editor's palette is nomagique's own primitives, described by the
hub from the library's declarations rather than from a list kept here. These
types mirror `nomagique/catalog`.Schema exactly; anything the editor wants to
know that the catalog does not carry is a change to the generator, not a
default invented on this side.
*/

/*
Port is one wired connection: a stream a primitive reads, or the one it
produces. Every primitive reads a run and answers with one, so `in` and `out`
are always present; the rest are the constructor's own stream arguments.
*/
export type Port = {
	name: string;
	type: string;
	description: string;
};

/*
Setting is a constructor argument that is typed in rather than wired, because
it is not itself a primitive.
*/
export type Setting = {
	name: string;
	type: string;
	description?: string;
};

export type Schema = {
	kind: string;
	category: string;
	/* The package and constructor that build this primitive, e.g. equation.Bound. */
	op: string;
	name: string;
	label: string;
	description: string;
	package: string;
	/* The constructor function that builds it, e.g. NewBound. */
	builder: string;
	/* Whether the last stream port accepts any number of connections. */
	variadic: boolean;
	inputs: Port[];
	outputs: Port[];
	config?: Setting[] | null;
};

export const fetchPrimitives = async (): Promise<Record<string, Schema>> => {
	const response = await fetch(`${hubBaseUrl()}/workbench/primitives`);

	if (!response.ok) {
		throw new Error(
			`Primitive catalog request failed (${response.status}): ${await response.text()}`,
		);
	}

	const primitives: Record<string, Schema> = await response.json();

	if (typeof primitives !== "object" || primitives === null) {
		throw new Error("The hub returned an invalid primitive catalog");
	}

	for (const [op, schema] of Object.entries(primitives)) {
		if (schema?.op !== op || !Array.isArray(schema.inputs)) {
			throw new Error(`The hub returned an invalid primitive: ${op}`);
		}
	}

	return primitives;
};

export type PrimitiveCatalog = {
	data?: Record<string, Schema>;
	isPending: boolean;
	isSuccess: boolean;
	isError: boolean;
	error?: Error;
};

/*
usePrimitives reads the catalog once. It is generated at build time from source
that cannot change under a running hub, so there is nothing to poll for and no
cache to invalidate — which is also why this is a plain fetch rather than a
query client: every other reader in this app fetches the same way, and one
call is not worth a second data-fetching paradigm.
*/
export function usePrimitives(): PrimitiveCatalog {
	const [state, setState] = useState<PrimitiveCatalog>({
		isPending: true,
		isSuccess: false,
		isError: false,
	});

	useEffect(() => {
		let live = true;

		fetchPrimitives()
			.then((data) => {
				if (live) {
					setState({
						data,
						isPending: false,
						isSuccess: true,
						isError: false,
					});
				}
			})
			.catch((error: Error) => {
				if (live) {
					setState({
						isPending: false,
						isSuccess: false,
						isError: true,
						error,
					});
				}
			});

		return () => {
			live = false;
		};
	}, []);

	return state;
}

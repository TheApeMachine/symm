import type { PortType } from "#/components/flume/types";

/*
A gathering port hands its callers numbered slots: metrics, metrics_1,
metrics_2 and so on are not separate ports but one port's slots. Drawn as a
row each, a grid collecting four hundred metrics is four hundred rows tall.

These group them back into the one port they belong to. The unsuffixed member
is the family's representative, so the handle an edge anchors to is a real port
rather than a heading invented for the occasion.
*/

/** The port a numbered slot belongs to, or null when it stands alone. */
export const portFamily = (name: string): string | null => {
	const match = /^(.+)_\d+$/.exec(name);

	return match ? match[1] : null;
};

export type PortGroup = {
	/** The port every member is a slot of. */
	base: string;
	/** The member an edge anchors to when the family is collapsed. */
	representative: PortType;
	members: PortType[];
};

/*
Groups a node's ports, keeping them in the order they were given. A port with
no numbered siblings comes back as a family of one, so callers have a single
shape to render.
*/
export const groupPorts = (ports: PortType[]): PortGroup[] => {
	const groups = new Map<string, PortGroup>();

	for (const port of ports) {
		const base = portFamily(port.name) ?? port.name;
		const existing = groups.get(base);

		if (!existing) {
			groups.set(base, { base, representative: port, members: [port] });
			continue;
		}

		existing.members.push(port);

		// The unsuffixed member is the port itself; prefer it as the anchor.
		if (port.name === base) {
			existing.representative = port;
		}
	}

	return [...groups.values()];
};

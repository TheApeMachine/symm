import { useAuth } from "@clerk/tanstack-react-start";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { NodeMap } from "#/components/flume";
import {
	backendAuthHeaders,
	backendBaseURL,
	type ClerkGetToken,
} from "#/lib/backend-http";

export type OperationPort = {
	name: string;
	type: string;
	description: string;
};

export type ConfigParam = {
	name: string;
	type: string;
	default?: unknown;
	description?: string;
};

export type TopologyNode = {
	id: string;
	op: string;
	in: string[];
	out: string[];
	config?: Record<string, unknown>;
	repeat?: number;
	index?: string;
	template?: TopologyNode[];
};

export type Schema = {
	kind: string;
	category: string;
	op: string;
	name: string;
	label: string;
	description: string;
	initial_width: number;
	inputs: OperationPort[];
	outputs: OperationPort[];
	config?: ConfigParam[] | null;
	system?: {
		topology: {
			nodes: TopologyNode[];
		};
	};
};

const fetchSchemas = async (
	kind: "operation" | "optimizer" | "block",
	getToken?: ClerkGetToken,
) => {
	const headers = await backendAuthHeaders(getToken);
	const response = await fetch(`${backendBaseURL()}/backend/compute/${kind}`, {
		headers,
	});

	return response.json() as Promise<Record<string, Schema>>;
};

export function useOperations() {
	const { getToken } = useAuth();

	return useQuery({
		queryKey: ["compute", "operation"],
		queryFn: () => fetchSchemas("operation", getToken),
	});
}

export function useOptimizers() {
	const { getToken } = useAuth();

	return useQuery({
		queryKey: ["compute", "optimizer"],
		queryFn: () => fetchSchemas("optimizer", getToken),
	});
}

export function useBlocks() {
	const { getToken } = useAuth();

	return useQuery({
		queryKey: ["compute", "block"],
		queryFn: () => fetchSchemas("block", getToken),
	});
}

export function useArchitectures() {
	const { getToken } = useAuth();

	return useQuery<string[]>({
		queryKey: ["architecture"],
		queryFn: async () => {
			const headers = await backendAuthHeaders(getToken);
			const response = await fetch(`${backendBaseURL()}/backend/architecture`, {
				headers,
			});

			return response.json();
		},
	});
}

export function useLoadArchitecture(name: string) {
	const { getToken } = useAuth();

	return useQuery<NodeMap>({
		queryKey: ["architecture", name],
		queryFn: async () => {
			const headers = await backendAuthHeaders(getToken);
			const response = await fetch(
				`${backendBaseURL()}/backend/architecture/${name}`,
				{ headers },
			);

			return response.json();
		},
		enabled: Boolean(name) && !name.startsWith("block.model."),
	});
}

export function useSaveArchitecture() {
	const queryClient = useQueryClient();
	const { getToken } = useAuth();

	return useMutation({
		mutationFn: async ({ name, nodes }: { name: string; nodes: NodeMap }) => {
			const headers = await backendAuthHeaders(getToken);
			headers.set("Content-Type", "application/json");

			const response = await fetch(
				`${backendBaseURL()}/backend/architecture/${name}`,
				{
					method: "POST",
					headers,
					body: JSON.stringify(nodes),
				},
			);

			return response.json();
		},
		onSuccess: () =>
			queryClient.invalidateQueries({ queryKey: ["architecture"] }),
	});
}

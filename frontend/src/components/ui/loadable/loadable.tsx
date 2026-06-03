"use client";

import type { ReactNode } from "react";
import { LoadableEmpty } from "#/components/ui/loadable/empty";
import { LoadableError } from "#/components/ui/loadable/error";
import { LoadablePending } from "#/components/ui/loadable/pending";

/*
LoadableProps describes the chrome states a data-driven view can be in.
Callers compute isLoading/isError/isEmpty from whatever source they
own — useLiveQuery, useQuery, custom hooks — and Loadable picks the
right surface to render. Slots override the defaults per-instance.
*/
export type LoadableProps = {
	name: string;
	isLoading: boolean;
	isError: boolean;
	isEmpty?: boolean;
	errorMessage?: string;
	pending?: ReactNode;
	error?: ReactNode;
	empty?: ReactNode;
	children: ReactNode;
};

/*
Loadable centralises the loading / error / empty / loaded chrome that
every query-driven panel was duplicating inline. It is fully chrome —
no useLiveQuery, no useQuery — so any data source plugs in.
*/
export const Loadable = ({
	name,
	isLoading,
	isError,
	isEmpty = false,
	errorMessage,
	pending,
	error,
	empty,
	children,
}: LoadableProps) => {
	if (isLoading) {
		return pending ?? <LoadablePending name={name} />;
	}

	if (isError) {
		return error ?? <LoadableError name={name} message={errorMessage} />;
	}

	if (isEmpty) {
		return empty ?? <LoadableEmpty name={name} />;
	}

	return children;
};

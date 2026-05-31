import { createContext, useContext, type ReactNode } from "react";

import {
	defaultSymmTelemetryStores,
	type SymmTelemetryStores,
} from "#/lib/symm/telemetry-stores";

const TelemetryContext = createContext<SymmTelemetryStores>(
	defaultSymmTelemetryStores,
);

export const SymmTelemetryProvider = ({
	stores = defaultSymmTelemetryStores,
	children,
}: {
	stores?: SymmTelemetryStores;
	children: ReactNode;
}) => (
	<TelemetryContext.Provider value={stores}>
		{children}
	</TelemetryContext.Provider>
);

export const useSymmTelemetryStores = () => useContext(TelemetryContext);

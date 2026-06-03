import {
	createContext,
	type ReactNode,
	useContext,
	useMemo,
	useState,
} from "react";

type WsStatusContextValue = {
	online: boolean;
	setOnline: (online: boolean) => void;
	balance: number;
	setBalance: (balance: number) => void;
};

const WsStatusContext = createContext<WsStatusContextValue | null>(null);

export const WsStatusProvider = ({ children }: { children: ReactNode }) => {
	const [online, setOnline] = useState(false);
	const [balance, setBalance] = useState(0);
	const value = useMemo(
		() => ({ online, setOnline, balance, setBalance }),
		[online, balance],
	);

	return (
		<WsStatusContext.Provider value={value}>
			{children}
		</WsStatusContext.Provider>
	);
};

export const useWsStatus = (): WsStatusContextValue => {
	const value = useContext(WsStatusContext);

	if (value === null) {
		throw new Error("useWsStatus must be used within WsStatusProvider");
	}

	return value;
};

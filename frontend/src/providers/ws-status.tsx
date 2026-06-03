import {
	createContext,
	type ReactNode,
	useCallback,
	useContext,
	useMemo,
	useState,
} from "react";

export type ActionEvent = {
	type: string;
	symbol: string;
	ts: number;
};

type WsStatusContextValue = {
	online: boolean;
	setOnline: (online: boolean) => void;
	balance: number;
	openPositions: number;
	setWallet: (balance: number, openPositions: number) => void;
	actions: ActionEvent[];
	pushAction: (action: ActionEvent) => void;
};

const WsStatusContext = createContext<WsStatusContextValue | null>(null);

export const WsStatusProvider = ({ children }: { children: ReactNode }) => {
	const [online, setOnline] = useState(false);
	const [balance, setBalance] = useState(0);
	const [openPositions, setOpenPositions] = useState(0);
	const [actions, setActions] = useState<ActionEvent[]>([]);

	const setWallet = useCallback((nextBalance: number, nextOpen: number) => {
		setBalance(nextBalance);
		setOpenPositions(nextOpen);
	}, []);

	const pushAction = useCallback((action: ActionEvent) => {
		setActions((prev) => [action, ...prev].slice(0, 50));
	}, []);

	const value = useMemo(
		() => ({
			online,
			setOnline,
			balance,
			openPositions,
			setWallet,
			actions,
			pushAction,
		}),
		[online, balance, openPositions, setWallet, actions, pushAction],
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

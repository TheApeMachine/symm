import {
	createContext,
	type ReactNode,
	useCallback,
	useContext,
	useMemo,
	useState,
} from "react";

export type ActionVerdict = "submitted" | "filled" | "rejected";

export type ActionEvent = {
	type: string;
	symbol: string;
	ts: number;
	verdict: ActionVerdict;
	reason: string;
};

export type Position = {
	symbol: string;
	qty: number;
	avgEntry: number;
};

export type PositionView = Position & {
	mark: number;
	unrealized: number;
	unrealizedPct: number;
};

type WsStatusContextValue = {
	online: boolean;
	setOnline: (online: boolean) => void;
	balance: number;
	openPositions: number;
	setWallet: (balance: number, openPositions: number) => void;
	positions: Position[];
	setPositions: (positions: Position[]) => void;
	marks: Record<string, number>;
	setMark: (symbol: string, price: number) => void;
	positionViews: PositionView[];
	actions: ActionEvent[];
	pushAction: (action: ActionEvent) => void;
};

const WsStatusContext = createContext<WsStatusContextValue | null>(null);

export const WsStatusProvider = ({ children }: { children: ReactNode }) => {
	const [online, setOnline] = useState(false);
	const [balance, setBalance] = useState(0);
	const [openPositions, setOpenPositions] = useState(0);
	const [positions, setPositions] = useState<Position[]>([]);
	const [marks, setMarks] = useState<Record<string, number>>({});
	const [actions, setActions] = useState<ActionEvent[]>([]);

	const setWallet = useCallback((nextBalance: number, nextOpen: number) => {
		setBalance(nextBalance);
		setOpenPositions(nextOpen);
	}, []);

	const setMark = useCallback((symbol: string, price: number) => {
		setMarks((prev) => (prev[symbol] === price ? prev : { ...prev, [symbol]: price }));
	}, []);

	// One card per symbol: the latest verdict replaces any prior card for that
	// symbol and moves to the top, so the panel reads as live per-symbol status
	// ("why is this not trading") instead of an unbounded flood of repeats.
	const pushAction = useCallback((action: ActionEvent) => {
		setActions((prev) =>
			[action, ...prev.filter((entry) => entry.symbol !== action.symbol)].slice(
				0,
				50,
			),
		);
	}, []);

	const positionViews = useMemo<PositionView[]>(
		() =>
			positions.map((position) => {
				const mark = marks[position.symbol] ?? position.avgEntry;
				const unrealized = (mark - position.avgEntry) * position.qty;
				const cost = position.avgEntry * position.qty;

				return {
					...position,
					mark,
					unrealized,
					unrealizedPct: cost > 0 ? (unrealized / cost) * 100 : 0,
				};
			}),
		[positions, marks],
	);

	const value = useMemo(
		() => ({
			online,
			setOnline,
			balance,
			openPositions,
			setWallet,
			positions,
			setPositions,
			marks,
			setMark,
			positionViews,
			actions,
			pushAction,
		}),
		[
			online,
			balance,
			openPositions,
			setWallet,
			positions,
			marks,
			setMark,
			positionViews,
			actions,
			pushAction,
		],
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

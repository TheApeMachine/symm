import {
	createContext,
	type ReactNode,
	useCallback,
	useContext,
	useLayoutEffect,
	useMemo,
	useState,
} from "react";
import { wsDispatchRef } from "#/providers/ws-dispatch";

export type ActionVerdict = "submitted" | "filled" | "rejected";

export type ActionEvent = {
	type: string;
	symbol: string;
	key: string;
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
	currency: string;
	exitBalance: number | null;
	capitalBase: number;
	openPositions: number;
	setWallet: (balance: number) => void;
	setCurrency: (currency: string) => void;
	setEquity: (exitBalance: number, capitalBase: number) => void;
	positions: Position[];
	setPositions: (positions: Position[]) => void;
	marks: Record<string, number>;
	setMark: (symbol: string, price: number) => void;
	positionViews: PositionView[];
	actions: ActionEvent[];
	pushAction: (action: ActionEvent) => void;
	storyTicks: number;
	playbookEvaluations: number;
	setPlaybookStats: (stats: {
		storyTicks: number;
		evaluations: number;
	}) => void;
};

const WsStatusContext = createContext<WsStatusContextValue | null>(null);

export const WsStatusProvider = ({ children }: { children: ReactNode }) => {
	const [online, setOnline] = useState(false);
	const [balance, setBalance] = useState(0);
	const [currency, setCurrency] = useState(
		(import.meta.env.VITE_QUOTE_CURRENCY?.trim() || "USD").toUpperCase(),
	);
	const [exitBalance, setExitBalance] = useState<number | null>(null);
	const [capitalBase, setCapitalBase] = useState(0);
	const [positions, setPositions] = useState<Position[]>([]);
	const [marks, setMarks] = useState<Record<string, number>>({});
	const [actions, setActions] = useState<ActionEvent[]>([]);
	const [storyTicks, setStoryTicks] = useState(0);
	const [playbookEvaluations, setPlaybookEvaluations] = useState(0);

	// The wallet frame carries cash only; the open-position count is derived from the
	// trader's positions list (published in both paper and live) rather than a count
	// the paper balance frame happens to add but the live one does not — so it stays
	// consistent the moment the paper connection is swapped for the live one.
	const setWallet = useCallback((nextBalance: number) => {
		setBalance(nextBalance);
	}, []);

	const setCurrencyCode = useCallback((nextCurrency: string) => {
		setCurrency(nextCurrency.toUpperCase());
	}, []);

	const setEquity = useCallback(
		(nextExitBalance: number, nextCapitalBase: number) => {
			setExitBalance(nextExitBalance);
			setCapitalBase(nextCapitalBase);
		},
		[],
	);

	const setMark = useCallback((symbol: string, price: number) => {
		setMarks((prev) =>
			prev[symbol] === price ? prev : { ...prev, [symbol]: price },
		);
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

	const setPlaybookStats = useCallback(
		(stats: { storyTicks: number; evaluations: number }) => {
			setStoryTicks(stats.storyTicks);
			setPlaybookEvaluations(stats.evaluations);
		},
		[],
	);

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

	useLayoutEffect(() => {
		wsDispatchRef.current = {
			setOnline,
			setWallet,
			setCurrency: setCurrencyCode,
			setEquity,
			setPositions,
			setMark,
			pushAction,
			setPlaybookStats,
		};
	});

	const value = useMemo(
		() => ({
			online,
			setOnline,
			balance,
			currency,
			exitBalance,
			capitalBase,
			openPositions: positions.length,
			setWallet,
			setCurrency: setCurrencyCode,
			setEquity,
			positions,
			setPositions,
			marks,
			setMark,
			positionViews,
			actions,
			pushAction,
			storyTicks,
			playbookEvaluations,
			setPlaybookStats,
		}),
		[
			online,
			balance,
			currency,
			exitBalance,
			capitalBase,
			setWallet,
			setCurrencyCode,
			setEquity,
			positions,
			marks,
			setMark,
			positionViews,
			actions,
			pushAction,
			storyTicks,
			playbookEvaluations,
			setPlaybookStats,
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

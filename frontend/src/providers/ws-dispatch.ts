import type { ActionEvent, Position } from "#/providers/ws-status";

export type WsDispatch = {
	setOnline: (online: boolean) => void;
	setWallet: (balance: number) => void;
	setCurrency: (currency: string) => void;
	setEquity: (exitBalance: number, capitalBase: number) => void;
	setPositions: (positions: Position[]) => void;
	setMark: (symbol: string, price: number) => void;
	pushAction: (action: ActionEvent) => void;
};

export const wsDispatchRef: { current: WsDispatch | null } = { current: null };

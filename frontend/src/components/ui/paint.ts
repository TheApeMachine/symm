export type JSONPrimitive = string | number | boolean | null;

export type JSONSerializable =
	| JSONPrimitive
	| JSONSerializable[]
	| { [key: string]: JSONSerializable | undefined };

export type Paint = (updates: JSONSerializable) => void;
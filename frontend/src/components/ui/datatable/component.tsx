import {
	type Context,
	type InitialQueryBuilder,
	type QueryBuilder,
	useLiveQuery,
} from "@tanstack/react-db";
// Context is used as a type bound in CollectionTableProps — keep this import.
import {
	type ColumnDef,
	flexRender,
	getCoreRowModel,
	getPaginationRowModel,
	getSortedRowModel,
	type PaginationState,
	type SortingState,
	useReactTable,
} from "@tanstack/react-table";
import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Frame } from "@/components/ui/frame";
import { Loadable } from "@/components/ui/loadable";
import { Table, TableBody, TableCell, TableRow } from "@/components/ui/table";
import { cn } from "@/lib/utils";
import { DataTableFooter } from "./footer";
import { DataTableHeader } from "./header";

type CellRenderer<TData> = (value: unknown, row: TData) => React.ReactNode;

export interface ColumnOverride<TData> {
	key: keyof TData;
	header?: string;
	size?: number;
	render?: CellRenderer<TData>;
}

export interface ExtraColumn<TData> {
	id: string;
	header?: string;
	size?: number;
	render: (row: TData) => React.ReactNode;
}

export interface DataTableProps<TData extends object> {
	data: TData[];
	columns?: (keyof TData)[];
	pageSize?: number;
	selectable?: boolean;
	defaultSortKey?: keyof TData;
	defaultSortDesc?: boolean;
	columnOverrides?: ColumnOverride<TData>[];
	extraColumns?: ExtraColumn<TData>[];
}

export interface CollectionTableProps<TData extends object>
	extends Omit<DataTableProps<TData>, "data"> {
	query: (q: InitialQueryBuilder) => QueryBuilder<Context>;
	errorMessage?: string;
}

const STATUS_COLORS: Record<string, string> = {
	active: "bg-emerald-500",
	inactive: "bg-amber-500",
	error: "bg-red-500",
	pending: "bg-blue-500",
};

const looksLikeStatus = (key: string) =>
	["status", "state", "health"].includes(key.toLowerCase());

const looksLikeDate = (value: unknown): value is string =>
	typeof value === "string" && !Number.isNaN(Date.parse(value));

const looksLikeBoolean = (value: unknown): value is boolean =>
	typeof value === "boolean";

function inferCellRenderer<TData>(
	key: string,
	sampleValue: unknown,
): CellRenderer<TData> {
	if (looksLikeStatus(key)) {
		return (value) => {
			const str = String(value).toLowerCase();
			return (
				<Badge variant="outline">
					<span
						aria-hidden="true"
						className={cn(
							"size-1.5 rounded-full",
							STATUS_COLORS[str] ?? "bg-muted-foreground/64",
						)}
					/>
					{String(value)}
				</Badge>
			);
		};
	}

	if (looksLikeBoolean(sampleValue)) {
		return (value) => (
			<Badge variant={value ? "default" : "outline"}>
				{value ? "Yes" : "No"}
			</Badge>
		);
	}

	if (looksLikeDate(sampleValue)) {
		return (value) => {
			const date = new Date(String(value));
			return (
				<span className="tabular-nums text-muted-foreground">
					{date.toLocaleDateString(undefined, {
						day: "2-digit",
						month: "short",
						year: "numeric",
					})}
				</span>
			);
		};
	}

	if (typeof sampleValue === "number") {
		return (value) => (
			<span className="tabular-nums font-mono">{String(value)}</span>
		);
	}

	return (value) => <span>{String(value ?? "")}</span>;
}

function toHeaderLabel(key: string): string {
	return key
		.replace(/([A-Z])/g, " $1")
		.replace(/_/g, " ")
		.replace(/^\s/, "")
		.replace(/\b\w/g, (c) => c.toUpperCase());
}

function buildColumns<TData extends object>(
	data: TData[],
	selectable: boolean,
	overrides: ColumnOverride<TData>[],
	extras: ExtraColumn<TData>[],
	allowedKeys?: (keyof TData)[],
): ColumnDef<TData>[] {
	const sample = data[0];
	const allKeys = Object.keys(sample) as (keyof TData)[];
	const keys = allowedKeys ?? allKeys;

	const overrideMap = new Map(overrides.map((col) => [col.key, col]));

	const dataCols: ColumnDef<TData>[] = keys.map((key) => {
		const override = overrideMap.get(key);
		const sampleValue = sample[key];
		const render =
			override?.render ?? inferCellRenderer<TData>(String(key), sampleValue);

		return {
			accessorKey: key as string,
			cell: ({ row }) => render(row.getValue(key as string), row.original),
			header: override?.header ?? toHeaderLabel(String(key)),
			size: override?.size,
		};
	});

	const extraCols: ColumnDef<TData>[] = extras.map((extra) => ({
		cell: ({ row }) => extra.render(row.original),
		enableSorting: false,
		header: extra.header ?? "",
		id: extra.id,
		size: extra.size,
	}));

	if (!selectable) return [...dataCols, ...extraCols];

	const selectCol: ColumnDef<TData> = {
		cell: ({ row }) => (
			<Checkbox
				aria-label="Select row"
				checked={row.getIsSelected()}
				onCheckedChange={(value) => row.toggleSelected(!!value)}
			/>
		),
		enableSorting: false,
		header: ({ table }) => {
			const isAllSelected = table.getIsAllPageRowsSelected();
			const isSomeSelected = table.getIsSomePageRowsSelected();
			return (
				<Checkbox
					aria-label="Select all rows"
					checked={isAllSelected}
					indeterminate={isSomeSelected && !isAllSelected}
					onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
				/>
			);
		},
		id: "select",
		size: 28,
	};

	return [selectCol, ...dataCols, ...extraCols];
}

const EmptyTable = () => (
	<Frame className="w-full">
		<Table variant="card">
			<TableBody>
				<TableRow>
					<TableCell className="h-24 text-center text-muted-foreground">
						No results.
					</TableCell>
				</TableRow>
			</TableBody>
		</Table>
	</Frame>
);

export function CollectionTable<TData extends object>({
	query,
	errorMessage,
	...props
}: CollectionTableProps<TData>) {
	const { data, isLoading, isError } = useLiveQuery(query);
	const rows = (data ?? []) as TData[];

	return (
		<Loadable
			name="data"
			isLoading={isLoading}
			isError={isError}
			isEmpty={!isLoading && !isError && rows.length === 0}
			errorMessage={errorMessage ?? "Failed to load data."}
			empty={<EmptyTable />}
		>
			<DataTable data={rows} {...props} />
		</Loadable>
	);
}

export const DataTable = <TData extends object>({
	data,
	columns: allowedKeys,
	pageSize = 10,
	selectable = false,
	defaultSortKey,
	defaultSortDesc = false,
	columnOverrides = [],
	extraColumns = [],
}: DataTableProps<TData>) => {
	const [pagination, setPagination] = useState<PaginationState>({
		pageIndex: 0,
		pageSize,
	});

	const [sorting, setSorting] = useState<SortingState>(
		defaultSortKey
			? [{ desc: defaultSortDesc, id: String(defaultSortKey) }]
			: [],
	);

	const columns = useMemo(
		() =>
			buildColumns(
				data,
				selectable,
				columnOverrides,
				extraColumns,
				allowedKeys,
			),
		[data, selectable, columnOverrides, extraColumns, allowedKeys],
	);

	const table = useReactTable({
		columns,
		data,
		enableSortingRemoval: false,
		getCoreRowModel: getCoreRowModel(),
		getPaginationRowModel: getPaginationRowModel(),
		getSortedRowModel: getSortedRowModel(),
		onPaginationChange: setPagination,
		onSortingChange: setSorting,
		state: { pagination, sorting },
	});

	return (
		<Frame className="w-full">
			<Table variant="card" className="table-fixed">
				<DataTableHeader table={table} />
				<TableBody>
					{table.getRowModel().rows.length ? (
						table.getRowModel().rows.map((row) => (
							<TableRow
								data-state={row.getIsSelected() ? "selected" : undefined}
								key={row.id}
							>
								{row.getVisibleCells().map((cell) => (
									<TableCell key={cell.id}>
										{flexRender(cell.column.columnDef.cell, cell.getContext())}
									</TableCell>
								))}
							</TableRow>
						))
					) : (
						<TableRow>
							<TableCell className="h-24 text-center" colSpan={columns.length}>
								No results.
							</TableCell>
						</TableRow>
					)}
				</TableBody>
			</Table>
			<DataTableFooter table={table} />
		</Frame>
	);
};

export default DataTable;

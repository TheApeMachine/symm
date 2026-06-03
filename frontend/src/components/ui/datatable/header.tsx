import { flexRender, type Table } from "@tanstack/react-table";
import { ChevronDownIcon, ChevronUpIcon } from "lucide-react";
import { TableHead, TableHeader, TableRow } from "../table";

interface DataTableHeaderProps<TData> {
	table: Table<TData>;
}

export const DataTableHeader = <TData,>({
	table,
}: DataTableHeaderProps<TData>) => {
	return (
		<TableHeader>
			{table.getHeaderGroups().map((headerGroup) => (
				<TableRow className="hover:bg-transparent" key={headerGroup.id}>
					{headerGroup.headers.map((header) => {
						const columnSize = header.column.getSize();
						return (
							<TableHead
								key={header.id}
								style={columnSize ? { width: `${columnSize}px` } : undefined}
							>
								{header.isPlaceholder ? null : header.column.getCanSort() ? (
									<button
										className="flex h-full w-full cursor-pointer select-none items-center justify-between gap-2 border-0 bg-transparent p-0 text-inherit"
										onClick={header.column.getToggleSortingHandler()}
										type="button"
									>
										{flexRender(
											header.column.columnDef.header,
											header.getContext(),
										)}
										{{
											asc: (
												<ChevronUpIcon
													aria-hidden="true"
													className="size-4 shrink-0 opacity-80"
												/>
											),
											desc: (
												<ChevronDownIcon
													aria-hidden="true"
													className="size-4 shrink-0 opacity-80"
												/>
											),
										}[header.column.getIsSorted() as string] ?? null}
									</button>
								) : (
									flexRender(
										header.column.columnDef.header,
										header.getContext(),
									)
								)}
							</TableHead>
						);
					})}
				</TableRow>
			))}
		</TableHeader>
	);
};

import type { Table } from "@tanstack/react-table";
import { Button } from "../button";
import { FrameFooter } from "../frame";
import {
	Pagination,
	PaginationContent,
	PaginationItem,
	PaginationNext,
	PaginationPrevious,
} from "../pagination";
import {
	Select,
	SelectItem,
	SelectPopup,
	SelectTrigger,
	SelectValue,
} from "../select";

interface DataTableFooterProp<TData> {
	table: Table<TData>;
}

export const DataTableFooter = <TData,>({
	table,
}: DataTableFooterProp<TData>) => {
	return (
		<FrameFooter className="p-2">
			<div className="flex items-center justify-between gap-2">
				{/* Results range selector */}
				<div className="flex items-center gap-2 whitespace-nowrap">
					<p className="text-muted-foreground text-sm">Viewing</p>
					<Select
						items={Array.from({ length: table.getPageCount() }, (_, i) => {
							const start = i * table.getState().pagination.pageSize + 1;
							const end = Math.min(
								(i + 1) * table.getState().pagination.pageSize,
								table.getRowCount(),
							);
							const pageNum = i + 1;
							return { label: `${start}-${end}`, value: pageNum };
						})}
						onValueChange={(value) => {
							table.setPageIndex((value as number) - 1);
						}}
						value={table.getState().pagination.pageIndex + 1}
					>
						<SelectTrigger
							aria-label="Select result range"
							className="w-fit min-w-none"
							size="sm"
						>
							<SelectValue />
						</SelectTrigger>
						<SelectPopup>
							{Array.from({ length: table.getPageCount() }, (_, i) => {
								const start = i * table.getState().pagination.pageSize + 1;
								const end = Math.min(
									(i + 1) * table.getState().pagination.pageSize,
									table.getRowCount(),
								);
								const pageNum = i + 1;
								return (
									<SelectItem key={pageNum} value={pageNum}>
										{`${start}-${end}`}
									</SelectItem>
								);
							})}
						</SelectPopup>
					</Select>
					<p className="text-muted-foreground text-sm">
						of{" "}
						<strong className="font-medium text-foreground">
							{table.getRowCount()}
						</strong>{" "}
						results
					</p>
				</div>
				{/* Pagination */}
				<Pagination className="justify-end">
					<PaginationContent>
						<PaginationItem>
							<PaginationPrevious
								className="sm:*:[svg]:hidden"
								render={
									<Button
										disabled={!table.getCanPreviousPage()}
										onClick={() => table.previousPage()}
										size="sm"
										variant="outline"
									/>
								}
							/>
						</PaginationItem>
						<PaginationItem>
							<PaginationNext
								className="sm:*:[svg]:hidden"
								render={
									<Button
										disabled={!table.getCanNextPage()}
										onClick={() => table.nextPage()}
										size="sm"
										variant="outline"
									/>
								}
							/>
						</PaginationItem>
					</PaginationContent>
				</Pagination>
			</div>
		</FrameFooter>
	);
};

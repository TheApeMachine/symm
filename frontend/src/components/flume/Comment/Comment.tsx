import React, { type ChangeEventHandler, type RefObject } from "react";
import { createPortal } from "react-dom";
import {
	type CommentAction,
	CommentActionTypes,
} from "#/components/flume/commentsReducer";
import { StageContext } from "#/components/flume/context";
import type {
	Colors,
	Coordinate,
	SelectOption,
	StageState,
} from "#/components/flume/types";
import { ScrollArea } from "#/components/ui/scroll-area";
import { Textarea } from "#/components/ui/textarea";
import ColorPicker from "../ColorPicker/ColorPicker";
import ContextMenu from "../ContextMenu/ContextMenu";
import Draggable from "../Draggable/Draggable";
import styles from "./Comment.module.css";

interface CommentProps {
	dispatch: React.Dispatch<CommentAction>;
	id: string;
	x: number;
	y: number;
	width: number;
	height: number;
	color: Colors;
	text: string;
	stageRect: RefObject<DOMRect | undefined>;
	onDragStart: () => void;
	isNew: boolean;
}

const Comment = ({
	dispatch,
	id,
	x,
	y,
	width,
	height,
	color,
	text,
	stageRect,
	onDragStart,
	isNew,
}: CommentProps) => {
	const stageState = React.useContext(StageContext) as StageState;
	const wrapper = React.useRef<HTMLDivElement>(null);
	const [isEditing, setIsEditing] = React.useState(false);
	const [isPickingColor, setIsPickingColor] = React.useState(false);
	const [menuOpen, setMenuOpen] = React.useState(false);
	const [menuCoordinates, setMenuCoordinates] = React.useState({
		x: 0,
		y: 0,
	});
	const [colorPickerCoordinates, setColorPickerCoordinates] = React.useState({
		x: 0,
		y: 0,
	});

	const handleContextMenu: React.MouseEventHandler<HTMLDivElement> = (e) => {
		e.preventDefault();
		e.stopPropagation();
		setMenuCoordinates({ x: e.clientX, y: e.clientY });
		setMenuOpen(true);
		return false;
	};

	const closeContextMenu = () => setMenuOpen(false);

	const startDrag = (_e: MouseEvent | TouchEvent) => {
		onDragStart();
	};

	const handleDrag = ({ x, y }: Coordinate) => {
		if (wrapper.current) {
			wrapper.current.style.transform = `translate(${x}px,${y}px)`;
		}
	};

	const handleDragEnd = (_e: MouseEvent, { x, y }: Coordinate) => {
		dispatch({
			type: CommentActionTypes.SET_COMMENT_COORDINATES,
			id,
			x,
			y,
		});
	};

	const handleResize = (coordinates: Coordinate) => {
		const width = Math.min(10000, Math.max(80, coordinates.x - x + 10));
		const height = Math.min(10000, Math.max(30, coordinates.y - y + 10));
		if (wrapper.current) {
			wrapper.current.style.width = `${width}px`;
			wrapper.current.style.height = `${height}px`;
		}
	};

	const handleResizeEnd = (_e: MouseEvent, coordinates: Coordinate) => {
		const width = Math.min(10000, Math.max(80, coordinates.x - x + 10));
		const height = Math.min(10000, Math.max(30, coordinates.y - y + 10));
		dispatch({
			type: CommentActionTypes.SET_COMMENT_DIMENSIONS,
			id,
			width,
			height,
		});
	};

	const handleMenuOption = (option: SelectOption) => {
		switch (option.value) {
			case "edit":
				startTextEdit();
				break;
			case "color":
				setColorPickerCoordinates(menuCoordinates);
				setIsPickingColor(true);
				break;
			case "delete":
				dispatch({
					type: CommentActionTypes.DELETE_COMMENT,
					id,
				});
				break;
			default:
		}
	};

	const startTextEdit = () => {
		setIsEditing(true);
	};

	const endTextEdit = () => {
		setIsEditing(false);
	};

	const handleTextChange: ChangeEventHandler<HTMLTextAreaElement> = (e) => {
		dispatch({
			type: CommentActionTypes.SET_COMMENT_TEXT,
			id,
			text: e.target.value,
		});
	};

	const handleColorPicked = (color: Colors) => {
		dispatch({
			type: CommentActionTypes.SET_COMMENT_COLOR,
			id,
			color,
		});
	};

	const handleRequestClose = React.useCallback(
		() => setIsPickingColor(false),
		[],
	);

	React.useEffect(() => {
		if (isNew) {
			setIsEditing(true);
			dispatch({
				type: CommentActionTypes.REMOVE_COMMENT_NEW,
				id,
			});
		}
	}, [isNew, dispatch, id]);

	const portalContainer =
		typeof document !== "undefined" ? document.body : null;

	return (
		<Draggable
			innerRef={wrapper}
			className={styles.wrapper}
			style={{
				transform: `translate(${x}px,${y}px)`,
				width,
				height,
				zIndex: isEditing ? 999 : "",
			}}
			stageState={stageState}
			stageRect={stageRect}
			onDragStart={startDrag}
			onDrag={handleDrag}
			onDragEnd={handleDragEnd}
			onContextMenu={handleContextMenu}
			onDoubleClick={startTextEdit}
			onWheel={(e) => e.stopPropagation()}
			data-color={color}
			data-flume-component="comment"
		>
			{isEditing ? (
				<Textarea
					unstyled
					data-flume-component="comment-textarea"
					className={styles.textareaWrapper}
					onChange={handleTextChange}
					onMouseDown={(e) => e.stopPropagation()}
					onBlur={endTextEdit}
					placeholder="Text of the comment..."
					value={text}
				/>
			) : (
				<ScrollArea className={styles.text}>
					<div
						data-flume-component="comment-text"
						data-comment={true}
						className={styles.textInner}
					>
						{text}
					</div>
				</ScrollArea>
			)}
			<Draggable
				className={styles.resizeThumb}
				stageState={stageState}
				stageRect={stageRect}
				onDrag={handleResize}
				onDragEnd={handleResizeEnd}
				data-flume-component="comment-resize-handle"
			/>
			{portalContainer && menuOpen
				? createPortal(
						<ContextMenu
							hideFilter
							label="Comment Options"
							x={menuCoordinates.x}
							y={menuCoordinates.y}
							options={[
								{
									value: "edit",
									label: "Edit Comment",
									description: "Edit the text of the comment",
								},
								{
									value: "color",
									label: "Change Color",
									description: "Change the color of the comment",
								},
								{
									value: "delete",
									label: "Delete Comment",
									description: "Delete the comment",
								},
							]}
							onRequestClose={closeContextMenu}
							onOptionSelected={handleMenuOption}
						/>,
						portalContainer,
					)
				: null}
			{portalContainer && isPickingColor
				? createPortal(
						<ColorPicker
							x={colorPickerCoordinates.x}
							y={colorPickerCoordinates.y}
							onRequestClose={handleRequestClose}
							onColorPicked={handleColorPicked}
						/>,
						portalContainer,
					)
				: null}
		</Draggable>
	);
};

export default Comment;

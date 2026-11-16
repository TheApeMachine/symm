/** Stable palette for open-choice mark paths — legend swatches use the same hash. */
const STROKES = [
	"#79D2C6",
	"#FF7DFD",
	"#5B88FF",
	"#D1765F",
	"#F5F77F",
	"#F8CFF7",
	"#6CA261",
	"#F1B470",
	"#B3A0D4",
	"#80F078",
];

export const strokeFor = (id: string) => {
	let h = 0;
	for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) | 0;
	return STROKES[Math.abs(h) % STROKES.length];
};

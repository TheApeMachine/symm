import { ElementSize, ListReader, MessageReader, PointerTag, StructReader } from '../frontend/node_modules/@naeemo/capnp/dist/index.js';

StructReader.prototype.getList = function (
	pointerIndex,
	_elementSize,
	structSize,
) {
	const ptrOffset = this.wordOffset + this.dataWords + pointerIndex;
	const resolved = this.message.resolvePointer(this.segmentIndex, ptrOffset);
	if (!resolved) return undefined;
	const { segmentIndex, wordOffset, pointer } = resolved;
	if (pointer.tag !== PointerTag.LIST) return undefined;
	const listPtr = pointer;
	if (listPtr.elementSize === undefined || listPtr.elementCount === undefined) {
		throw new Error("Cap'n Proto list is missing its element size or count");
	}
	let targetOffset = wordOffset;
	let elementCount = listPtr.elementCount;
	let actualStructSize = structSize;
	const segment = this.message.getSegment(segmentIndex);
	if (listPtr.elementSize === ElementSize.COMPOSITE) {
		if (targetOffset < 0 || !segment || targetOffset >= segment.wordCount)
			return undefined;
		try {
			const tagWord = segment.getWord(targetOffset);
			elementCount = Number((tagWord >> 2n) & 0x3fffffffn);
			actualStructSize = {
				dataWords: Number((tagWord >> 32n) & 65535n),
				pointerCount: Number((tagWord >> 48n) & 65535n),
			};
			targetOffset += 1;
		} catch {
			return undefined;
		}
	}
	return new ListReader(
		this.message,
		segmentIndex,
		listPtr.elementSize,
		elementCount,
		actualStructSize,
		targetOffset,
	);
};

function readBoundTexts(buffer) {
    const message = new MessageReader(buffer);
    const root = message.getRoot(0, 1);
    if (!root) return [];
    const list = root.getList(0);
    if (!list) return [];
    const out = [];
    for (let index = 0; index < list.length; index++) {
        const bound = list.getStruct(index);
        if (!bound) continue;
        out.push({
            graph: bound.getText(0) ?? "",
            component: bound.getText(1) ?? "",
            prop: bound.getText(2) ?? "",
            text: bound.getText(3) ?? "",
        });
    }
    return out;
}

const ws = new WebSocket('ws://127.0.0.1:8765/ws');

const seenComponents = new Map();

ws.addEventListener('open', () => {
    console.log('Connected to ws://127.0.0.1:8765/ws');
    ws.send(JSON.stringify({ type: "route", route: "dashboard" }));
    ws.send(JSON.stringify({ type: "focus", symbol: "BTC/USD" }));
});

ws.addEventListener('message', async (event) => {
    console.log('WS message received, type:', typeof event.data, 'size:', event.data?.byteLength ?? event.data?.length);
    const buf = (event.data instanceof ArrayBuffer) ? event.data : (event.data.buffer ?? (await event.data.arrayBuffer()));
    try {
        const bounds = readBoundTexts(buf);
        for (const b of bounds) {
            const key = `${b.graph}/${b.component}/${b.prop}`;
            seenComponents.set(key, b.text);
            if (b.prop === 'points' && b.component === 'impulse') {
                try {
                    const parsed = JSON.parse(b.text);
                    console.log(`[IMPULSE POINTS] total points: ${parsed.length}`);
                    if (parsed.length > 0) {
                        const sample = parsed.slice(0, 5);
                        console.log(`[IMPULSE POINTS SAMPLE]`, JSON.stringify(sample));
                        const labels = new Set(parsed.map(p => p.label || p.source));
                        console.log(`[IMPULSE LABELS] ${labels.size} unique:`, Array.from(labels).slice(0, 10));
                    }
                } catch (e) {}
            }
            if (b.prop === 'regions') {
                console.log(`[REGIONS] ${key}: ${b.text}`);
            }
            if (b.prop === 'connections') {
                console.log(`[CONNECTIONS] ${key}: ${b.text.slice(0, 150)}`);
            }
            if (b.component === 'decisionList') {
                console.log(`[DECISION LIST] ${key}: ${b.text}`);
            }
            if (b.component === 'positionList') {
                console.log(`[POSITION LIST] ${key}: ${b.text}`);
            }
        }
    } catch (e) {
        // ignore decode errors for other frame types
    }
});

setTimeout(() => {
    console.log('\n--- SUMMARY OF ALL SEEN BOUND KEYS ---');
    for (const [k, v] of seenComponents.entries()) {
        console.log(`${k} -> ${v.slice(0, 80)}`);
    }
    ws.close();
    process.exit(0);
}, 15000);

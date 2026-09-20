import { WebSocket } from 'ws';
import { MessageReader } from '@naeemo/capnp';

const ws = new WebSocket('ws://127.0.0.1:8765/ws');
ws.binaryType = 'arraybuffer';

ws.on('message', (data) => {
    console.log('Received message of size', data.byteLength);
    try {
        const reader = new MessageReader(data);
        const root = reader.getRoot(7, 4); // 7 words, 4 pointers
        
        const tick = root.getInt64(8);
        console.log('Tick:', tick);
        
        const labelStr = root.getData(1); // pointer 1 is label Data
        const label = new TextDecoder().decode(labelStr);
        console.log('Label:', label);
        
        const entity = root.getUint16(24);
        const source = root.getUint16(26);
        console.log('Entity:', entity, 'Source:', source);
        
        process.exit(0);
    } catch (e) {
        console.error('Error parsing:', e);
        process.exit(1);
    }
});

ws.on('open', () => {
    console.log('Connected');
});

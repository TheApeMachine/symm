package data

import (
	"encoding/binary"
	"errors"
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

const MeasurementMagic = "SYMM\x01"

/*
MarshalBinary serializes the Measurement into high-performance binary format,
avoiding all JSON serialization tax and retaining exact IEEE 754 precision,
exact venue decimal precision, and structured metadata.
*/
func (measurement *Measurement) MarshalBinary() ([]byte, error) {
	if measurement == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[data.codec] cannot marshal nil measurement", nil,
		))
	}

	approxSize := max(measurement.ApproximateBytes(), 256)

	writer := newBinaryWriter(int(approxSize))

	// 1. Magic prefix
	writer.writeBytes([]byte(MeasurementMagic))

	// 2. Fixed numerical header
	writer.writeUint32(measurement.ID)
	writer.writeInt64(measurement.Epoch)
	writer.writeInt64(measurement.SeqIdx)
	writer.writeInt64(measurement.Tick)
	writer.writeInt64(measurement.Timestamp)

	var atNano int64

	if !measurement.At.IsZero() {
		atNano = measurement.At.UTC().UnixNano()
	}

	writer.writeInt64(atNano)

	var fromNano int64

	if !measurement.From.IsZero() {
		fromNano = measurement.From.UTC().UnixNano()
	}

	writer.writeInt64(fromNano)
	writer.writeFloat64(measurement.maturity)
	writer.writeInt64(measurement.samples)
	writer.writeFloat64(measurement.energy)
	writer.writeFloat64(measurement.prediction)

	// 3. String identifiers & error
	writer.writeString(measurement.Label)
	writer.writeString(measurement.Source)
	writer.writeError(measurement.err)

	// 4. Metrics
	writer.writeUint16(uint16(len(measurement.metrics)))

	for _, entry := range measurement.metrics {
		if entry == nil {
			writer.writeString("")
			writer.writeError(nil)
			writer.writeByte(0)
			continue
		}

		writer.writeString(entry.Key)
		writer.writeError(entry.Err)

		if entry.Metric == nil {
			writer.writeByte(0)
			continue
		}

		writer.writeByte(1)
		writer.writeString(entry.Metric.label)
		writer.writeFloat64(entry.Metric.raw)
		writer.writeFloat64(entry.Metric.snm)
		writer.writeString(string(entry.Metric.unit))
		writer.writeString(string(entry.Metric.timescale))
		writer.writeError(entry.Metric.err)

		if entry.Metric.exact == nil {
			writer.writeByte(0)
			continue
		}

		writer.writeByte(1)
		writer.writeString(entry.Metric.exact.String())
	}

	// 5. Metadata
	writer.writeUint16(uint16(len(measurement.metadata)))

	for _, entry := range measurement.metadata {
		if entry == nil {
			writer.writeString("")
			writer.writeString("")
			writer.writeError(nil)
			continue
		}

		writer.writeString(entry.Key)
		writer.writeString(entry.Value)
		writer.writeError(entry.Err)
	}

	// 6. Peers
	writer.writeUint16(uint16(len(measurement.peers)))

	for _, peer := range measurement.peers {
		if peer == nil {
			writer.writeUint32(0)
			continue
		}

		peerPayload, err := peer.MarshalBinary()

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Internal, "[data.codec] failed to marshal peer", err,
			))
		}

		writer.writeUint32(uint32(len(peerPayload)))
		writer.writeBytes(peerPayload)
	}

	return writer.buf, nil
}

/*
UnmarshalBinary deserializes the Measurement from high-performance binary format.
*/
func (measurement *Measurement) UnmarshalBinary(payload []byte) error {
	if measurement == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[data.codec] cannot unmarshal into nil measurement", nil,
		))
	}

	if len(payload) < len(MeasurementMagic) {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[data.codec] payload too short", nil,
		))
	}

	reader := newBinaryReader(payload)
	magic, err := reader.readBytes(len(MeasurementMagic))

	if err != nil {
		return err
	}

	if string(magic) != MeasurementMagic {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[data.codec] invalid magic prefix", nil,
		))
	}

	measurement.ID, err = reader.readUint32()

	if err != nil {
		return err
	}

	measurement.Epoch, err = reader.readInt64()

	if err != nil {
		return err
	}

	measurement.SeqIdx, err = reader.readInt64()

	if err != nil {
		return err
	}

	measurement.Tick, err = reader.readInt64()

	if err != nil {
		return err
	}

	measurement.Timestamp, err = reader.readInt64()

	if err != nil {
		return err
	}

	atNano, err := reader.readInt64()

	if err != nil {
		return err
	}

	if atNano != 0 {
		measurement.At = time.Unix(0, atNano).UTC()
	}

	fromNano, err := reader.readInt64()

	if err != nil {
		return err
	}

	if fromNano != 0 {
		measurement.From = time.Unix(0, fromNano).UTC()
	}

	measurement.maturity, err = reader.readFloat64()

	if err != nil {
		return err
	}

	measurement.samples, err = reader.readInt64()

	if err != nil {
		return err
	}

	measurement.energy, err = reader.readFloat64()

	if err != nil {
		return err
	}

	measurement.prediction, err = reader.readFloat64()

	if err != nil {
		return err
	}

	measurement.Label, err = reader.readString()

	if err != nil {
		return err
	}

	measurement.Source, err = reader.readString()

	if err != nil {
		return err
	}

	measurement.err, err = reader.readError()

	if err != nil {
		return err
	}

	// Metrics
	metricCount, err := reader.readUint16()

	if err != nil {
		return err
	}

	measurement.metrics = make([]*MetricEntry, 0, int(metricCount))

	for idx := 0; idx < int(metricCount); idx++ {
		metricKey, keyErr := reader.readString()

		if keyErr != nil {
			return keyErr
		}

		entryErr, readEntryErr := reader.readError()

		if readEntryErr != nil {
			return readEntryErr
		}

		hasMetric, flagErr := reader.readByte()

		if flagErr != nil {
			return flagErr
		}

		if hasMetric == 0 {
			measurement.metrics = append(measurement.metrics, &MetricEntry{
				Key: metricKey,
				Err: entryErr,
			})
			continue
		}

		label, labelErr := reader.readString()

		if labelErr != nil {
			return labelErr
		}

		raw, rawErr := reader.readFloat64()

		if rawErr != nil {
			return rawErr
		}

		snm, snmErr := reader.readFloat64()

		if snmErr != nil {
			return snmErr
		}

		unitStr, unitErr := reader.readString()

		if unitErr != nil {
			return unitErr
		}

		timescaleStr, timescaleErr := reader.readString()

		if timescaleErr != nil {
			return timescaleErr
		}

		metricErr, mErr := reader.readError()

		if mErr != nil {
			return mErr
		}

		hasExact, exactFlagErr := reader.readByte()

		if exactFlagErr != nil {
			return exactFlagErr
		}

		var exactDecimal *decimal.Decimal

		if hasExact == 1 {
			exactStr, exactStrErr := reader.readString()

			if exactStrErr != nil {
				return exactStrErr
			}

			parsedDecimal, parseErr := decimal.NewFromString(exactStr)

			if parseErr != nil {
				return errnie.Error(errnie.Err(
					errnie.Validation, "[data.codec] invalid exact decimal string", parseErr,
				))
			}

			exactDecimal = parsedDecimal
		}

		metric := &Metric{
			err:       metricErr,
			label:     label,
			raw:       raw,
			snm:       snm,
			exact:     exactDecimal,
			unit:      Unit(unitStr),
			timescale: Timescale(timescaleStr),
		}

		measurement.metrics = append(measurement.metrics, &MetricEntry{
			Key:    metricKey,
			Metric: metric,
			Err:    entryErr,
		})
	}

	// Metadata
	metaCount, err := reader.readUint16()

	if err != nil {
		return err
	}

	measurement.metadata = make([]*StringEntry, 0, int(metaCount))

	for idx := 0; idx < int(metaCount); idx++ {
		metaKey, keyErr := reader.readString()

		if keyErr != nil {
			return keyErr
		}

		metaValue, valErr := reader.readString()

		if valErr != nil {
			return valErr
		}

		metaErr, errRead := reader.readError()

		if errRead != nil {
			return errRead
		}

		measurement.metadata = append(measurement.metadata, &StringEntry{
			Key:   metaKey,
			Value: metaValue,
			Err:   metaErr,
		})
	}

	// Peers
	peerCount, err := reader.readUint16()

	if err != nil {
		return err
	}

	measurement.peers = make([]*Measurement, 0, int(peerCount))

	for idx := 0; idx < int(peerCount); idx++ {
		peerLen, lenErr := reader.readUint32()

		if lenErr != nil {
			return lenErr
		}

		if peerLen == 0 {
			continue
		}

		peerData, dataErr := reader.readBytes(int(peerLen))

		if dataErr != nil {
			return dataErr
		}

		peer := &Measurement{}

		if unmarshalErr := peer.UnmarshalBinary(peerData); unmarshalErr != nil {
			return unmarshalErr
		}

		measurement.peers = append(measurement.peers, peer)
	}

	return nil
}

type binaryWriter struct {
	buf []byte
}

func newBinaryWriter(capacity int) *binaryWriter {
	return &binaryWriter{
		buf: make([]byte, 0, capacity),
	}
}

func (writer *binaryWriter) writeByte(val byte) {
	writer.buf = append(writer.buf, val)
}

func (writer *binaryWriter) writeUint16(val uint16) {
	var scratch [2]byte
	binary.LittleEndian.PutUint16(scratch[:], val)
	writer.buf = append(writer.buf, scratch[:]...)
}

func (writer *binaryWriter) writeUint32(val uint32) {
	var scratch [4]byte
	binary.LittleEndian.PutUint32(scratch[:], val)
	writer.buf = append(writer.buf, scratch[:]...)
}

func (writer *binaryWriter) writeUint64(val uint64) {
	var scratch [8]byte
	binary.LittleEndian.PutUint64(scratch[:], val)
	writer.buf = append(writer.buf, scratch[:]...)
}

func (writer *binaryWriter) writeInt64(val int64) {
	writer.writeUint64(uint64(val))
}

func (writer *binaryWriter) writeFloat64(val float64) {
	writer.writeUint64(math.Float64bits(val))
}

func (writer *binaryWriter) writeBytes(val []byte) {
	writer.buf = append(writer.buf, val...)
}

func (writer *binaryWriter) writeString(str string) {
	writer.writeUint16(uint16(len(str)))
	writer.buf = append(writer.buf, str...)
}

func (writer *binaryWriter) writeError(err error) {
	if err == nil {
		writer.writeUint16(0)
		return
	}

	errStr := err.Error()
	writer.writeUint16(uint16(len(errStr)))
	writer.buf = append(writer.buf, errStr...)
}

type binaryReader struct {
	data   []byte
	offset int
}

func newBinaryReader(data []byte) *binaryReader {
	return &binaryReader{
		data:   data,
		offset: 0,
	}
}

func (reader *binaryReader) readByte() (byte, error) {
	if reader.offset >= len(reader.data) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "[data.codec] unexpected EOF reading byte", nil,
		))
	}

	val := reader.data[reader.offset]
	reader.offset++
	return val, nil
}

func (reader *binaryReader) readUint16() (uint16, error) {
	if reader.offset+2 > len(reader.data) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "[data.codec] unexpected EOF reading uint16", nil,
		))
	}

	val := binary.LittleEndian.Uint16(reader.data[reader.offset:])
	reader.offset += 2
	return val, nil
}

func (reader *binaryReader) readUint32() (uint32, error) {
	if reader.offset+4 > len(reader.data) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "[data.codec] unexpected EOF reading uint32", nil,
		))
	}

	val := binary.LittleEndian.Uint32(reader.data[reader.offset:])
	reader.offset += 4
	return val, nil
}

func (reader *binaryReader) readUint64() (uint64, error) {
	if reader.offset+8 > len(reader.data) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "[data.codec] unexpected EOF reading uint64", nil,
		))
	}

	val := binary.LittleEndian.Uint64(reader.data[reader.offset:])
	reader.offset += 8
	return val, nil
}

func (reader *binaryReader) readInt64() (int64, error) {
	val, err := reader.readUint64()

	if err != nil {
		return 0, err
	}

	return int64(val), nil
}

func (reader *binaryReader) readFloat64() (float64, error) {
	val, err := reader.readUint64()

	if err != nil {
		return 0, err
	}

	return math.Float64frombits(val), nil
}

func (reader *binaryReader) readBytes(count int) ([]byte, error) {
	if count < 0 || reader.offset+count > len(reader.data) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[data.codec] unexpected EOF reading bytes", nil,
		))
	}

	val := reader.data[reader.offset : reader.offset+count]
	reader.offset += count
	return val, nil
}

func (reader *binaryReader) readString() (string, error) {
	strLen, err := reader.readUint16()

	if err != nil {
		return "", err
	}

	if strLen == 0 {
		return "", nil
	}

	strBytes, err := reader.readBytes(int(strLen))

	if err != nil {
		return "", err
	}

	return string(strBytes), nil
}

func (reader *binaryReader) readError() (error, error) {
	errLen, err := reader.readUint16()

	if err != nil {
		return nil, err
	}

	if errLen == 0 {
		return nil, nil
	}

	errBytes, err := reader.readBytes(int(errLen))

	if err != nil {
		return nil, err
	}

	return errors.New(string(errBytes)), nil
}

#!/usr/bin/env python3
"""One-time graph migration. The emitted JSON, not this script, is the program."""
from pathlib import Path
from collections import defaultdict
import json


def load(name):
    return json.loads(Path("manifest", name + ".json").read_text())


def add(graph, identifier, kind, **values):
    graph["nodes"][identifier] = {"id": identifier, "type": kind,
        "inputData": {key: {"value": value} for key, value in values.items()},
        "connections": {"inputs": {}, "outputs": {}}}


def wire(graph, source, output, target, port):
    refs = graph["nodes"][source]["connections"]["outputs"].setdefault(output, [])
    ref = {"nodeId": target, "portName": port}
    if ref not in refs:
        refs.append(ref)


def disconnect(graph, target, prefix=None):
    for node in graph["nodes"].values():
        for port, refs in node["connections"]["outputs"].items():
            node["connections"]["outputs"][port] = [ref for ref in refs if not
                (ref["nodeId"] == target and (prefix is None or ref["portName"].startswith(prefix)))]


def remove(graph, identifier):
    disconnect(graph, identifier)
    graph["nodes"].pop(identifier, None)


def move_output(graph, source, port, replacement, replacement_port):
    refs = graph["nodes"][source]["connections"]["outputs"].pop(port, [])
    for ref in refs:
        wire(graph, replacement, replacement_port, ref["nodeId"], ref["portName"])


def save(name, graph):
    for node in graph["nodes"].values():
        node["connections"]["inputs"] = {}
    for identifier, node in graph["nodes"].items():
        for port, refs in node["connections"]["outputs"].items():
            for ref in refs:
                target = graph["nodes"][ref["nodeId"]]
                target["connections"]["inputs"].setdefault(ref["portName"], []).append({"nodeId": identifier, "portName": port})
    Path("manifest", name + ".json").write_text(json.dumps(graph, indent=2) + "\n")


def extract(graph, identifier, source, output, path, encoding="json"):
    add(graph, identifier, "data.Extract", path=path, encoding=encoding)
    wire(graph, source, output, identifier, "data")


signals = load("signals")
if signals["nodes"]["gather"]["type"] != "store.Boundary":
    publications = defaultdict(list)
    for port, refs in signals["nodes"]["gather"]["connections"]["inputs"].items():
        if port != "values" and not port.startswith("values_"):
            continue
        coordinate = 0 if port == "values" else int(port[7:])
        if len(refs) != 1:
            raise RuntimeError("one writer is required for each original coordinate")
        publications[refs[0]["nodeId"]].append((coordinate, refs[0]["portName"]))
    remove(signals, "gather")
    roster = []
    for index, (owner, fields) in enumerate(sorted(publications.items())):
        fields.sort()
        coordinates = [coordinate for coordinate, _ in fields]
        roster.append({"producer": owner, "coordinates": coordinates})
        publisher = "publication_" + str(index)
        add(signals, publisher, "data.MeasurementService", producer=owner,
            coordinates=json.dumps(coordinates, separators=(",", ":")))
        for position, (_, output) in enumerate(fields):
            wire(signals, owner, output, publisher, "values_" + str(position))
        wire(signals, "grid", "run", publisher, "run")
        wire(signals, "grid", "sequence", publisher, "tick")
    add(signals, "gather", "store.Boundary", layout=json.dumps(roster, separators=(",", ":")))
    for index in range(len(roster)):
        wire(signals, "publication_" + str(index), "read", "gather", "publications_" + str(index))
    for port in ("run", "sequence", "receipt"):
        wire(signals, "grid", port, "gather", port)
    save("signals", signals)

# One raw frame receipt follows normalization and every record derived from it.
feed = {"id": "measurement_feed", "name": "Captured records with exact computation ordinals", "nodes": {}}
add(feed, "payload", "transport.Fan")
add(feed, "receipt", "transport.Fan")
add(feed, "captured", "data.Insert", path="capture")
wire(feed, "payload", "out", "captured", "data")
wire(feed, "receipt", "out", "captured", "json")
extract(feed, "received", "receipt", "out", "received_time", "text")
add(feed, "timed", "data.Insert", path="capture.receivedAt", encoding="text")
wire(feed, "captured", "out", "timed", "data")
wire(feed, "received", "text", "timed", "text")
add(feed, "spot", "data.Iterate", path="data", envelope=True, indexPath="capture.record")
wire(feed, "timed", "out", "spot", "data")
add(feed, "focus", "data.Filter", path="data.symbol", operator="==", value="BTC/USD")
# Filter's literal comparison port is threshold, not an inferred string alias.
feed["nodes"]["focus"]["inputData"].pop("value")
feed["nodes"]["focus"]["inputData"]["threshold"] = {"value": "BTC/USD"}
wire(feed, "spot", "out", "focus", "data")
add(feed, "futures", "kraken.Futures")
wire(feed, "timed", "out", "futures", "data")
add(feed, "records", "data.Iterate", path="")
wire(feed, "focus", "out", "records", "data_0")
wire(feed, "futures", "records", "records", "data_1")
extract(feed, "run", "records", "out", "capture.capture_session", "text")
add(feed, "stamp", "store.Stamp")
wire(feed, "records", "out", "stamp", "data")
wire(feed, "run", "text", "stamp", "run")
save("measurement_feed", feed)

# A causal receipt is indexed using the measurement clock, with the original
# raw receipt kept alongside it rather than substituting timestamps for IDs.
record = {"id": "measurement_record", "name": "Boundary-linked causal record", "nodes": {}}
add(record, "boundary", "transport.Fan")
add(record, "receipt", "transport.Fan")
extract(record, "run", "boundary", "out", "run")
extract(record, "sequence", "boundary", "out", "sequence")
extract(record, "endpoint", "receipt", "out", "capture.endpoint")
extract(record, "received", "receipt", "out", "capture.received_time")
extract(record, "raw", "receipt", "out", "capture")
add(record, "session", "data.Insert", path="capture.session", data='{"cursor":{"record":0}}')
wire(record, "run", "json", "session", "json")
previous = "session"
for identifier, path, source, output in (
    ("cursor", "cursor.sequence", "sequence", "json"),
    ("venue", "capture.endpoint", "endpoint", "json"),
    ("time", "capture.receivedAt", "received", "json"),
    ("original", "rawCapture", "raw", "json"),
    ("record", "market", "receipt", "out"),
):
    add(record, identifier, "data.Insert", path=path)
    wire(record, previous, "out", identifier, "data")
    wire(record, source, output, identifier, "json")
    previous = identifier
save("measurement_record", record)

boundary_fields = [{"id": index + 1, "name": name, "type": kind, "required": True}
    for index, (name, kind) in enumerate((("run", "string"), ("sequence", "long"),
        ("previous", "long"), ("digest", "string"), ("payload", "binary")))]

for root_name in ("system", "capture"):
    graph = load(root_name)
    if "measurement_archive" in graph["nodes"]:
        continue
    # Every existing physical Level3 connection feeds the same raw owner.
    level3 = load("live_level3")
    shards = sorted(identifier for identifier, entry in level3["nodes"].items()
        if entry["type"] == "definition:live_level3_shard")
    for position, shard in enumerate(shards[1:], start=3):
        for field, port in (("read", "payload"), ("endpoint", "endpoint"), ("receivedAt", "receivedAt")):
            wire(graph, "level3", shard + ".socket.frame." + field, "envelope", port + "_" + str(position))
    if "signals" not in graph["nodes"]:
        add(graph, "signals", "definition:signals")
    disconnect(graph, "signals", "grid.data")
    add(graph, "measurement_feed", "definition:measurement_feed")
    wire(graph, "envelope", "row.payload", "measurement_feed", "payload.data")
    wire(graph, "envelope", "row.out", "measurement_feed", "receipt.data")
    for output, target in (("data", "data"), ("data", "receipt"), ("run", "run"), ("sequence", "sequence")):
        wire(graph, "measurement_feed", "stamp.item." + output, "signals", "grid." + target)
    for port in list(graph["nodes"]["signals"]["connections"]["outputs"]):
        if port.startswith("gather.gathered."):
            move_output(graph, "signals", port, "signals", port.replace("gather.gathered.", "gather.ready."))
    config = json.loads(graph["nodes"]["capture"]["inputData"]["config"]["value"])
    config["table"], config["fields"] = "measurement_boundaries_v1", boundary_fields
    add(graph, "measurement_archive", "tables.IcebergTable", config=json.dumps(config, separators=(",", ":")))
    wire(graph, "signals", "gather.row", "measurement_archive", "payload")
    # In-band discovery consumes the same sealed boundary's venue observation.
    add(graph, "measurement_mine", "temporal.Mine", channel="ticker", priceField="last")
    extract(graph, "measurement_endpoint", "signals", "gather.receipt", "capture.endpoint", "text")
    wire(graph, "signals", "gather.receipt", "measurement_mine", "payload_0")
    wire(graph, "signals", "gather.sequence", "measurement_mine", "sequence_0")
    wire(graph, "signals", "gather.run", "measurement_mine", "session")
    wire(graph, "measurement_endpoint", "text", "measurement_mine", "endpoint_0")
    event_config = json.loads(load("training")["nodes"]["events"]["inputData"]["config"]["value"])
    event_config["table"] = "measurement_excursions_v1"
    add(graph, "measurement_events", "tables.IcebergTable", config=json.dumps(event_config, separators=(",", ":")))
    wire(graph, "measurement_mine", "events.out", "measurement_events", "rows")
    if "live_map" in graph["nodes"]:
        for identifier, entry in load("impulse_map")["nodes"].items():
            if entry["type"] == "store.Vector":
                wire(graph, "signals", "gather.run", "live_map", identifier + ".scope")
    save(root_name, graph)

training = load("training")
if "measured" not in training["nodes"]:
    raw_config = json.loads(training["nodes"]["replay"]["inputData"]["input.through"]["value"])
    raw_config["tableUrl"] = raw_config["tableUrl"].replace("raw_frames_v3", "measurement_boundaries_v1")
    add(training, "measured", "definition:measurement_replay", **{"archive.input.through": json.dumps(raw_config, separators=(",", ":"))})
    # Raw replay remains exclusively for instrument/book execution facts.
    # Signal calculations are not part of the training path anymore.
    remove(training, "signals")
    disconnect(training, "mine")
    extract(training, "measurement_endpoint", "measured", "boundary.receipt", "capture.endpoint", "text")
    wire(training, "measured", "boundary.receipt", "mine", "payload_0")
    wire(training, "measured", "boundary.sequence", "mine", "sequence_0")
    wire(training, "measured", "boundary.run", "mine", "session")
    wire(training, "measurement_endpoint", "text", "mine", "endpoint_0")
    disconnect(training, "impulse_map", "change.")
    disconnect(training, "impulse_map", "previous.scope")
    wire(training, "measured", "boundary.ready.values", "impulse_map", "change.value")
    wire(training, "measured", "boundary.ready.present", "impulse_map", "change.present")
    for identifier, entry in load("impulse_map")["nodes"].items():
        if entry["type"] == "store.Vector":
            wire(training, "measured", "boundary.run", "impulse_map", identifier + ".scope")
    add(training, "measurement_record", "definition:measurement_record")
    wire(training, "measured", "boundary.row", "measurement_record", "boundary.data")
    wire(training, "measured", "boundary.receipt", "measurement_record", "receipt.data")
    add(training, "recorded_tokens", "data.Insert", path="measured.tokens")
    add(training, "recorded_vocabulary", "data.Insert", path="measured.vocabulary")
    wire(training, "impulse_map", "hot.out", "recorded_tokens", "json")
    wire(training, "recorded_tokens", "out", "recorded_vocabulary", "data")
    wire(training, "impulse_map", "peak.settled.vocabulary", "recorded_vocabulary", "json")
    add(training, "recorded_reading", "data.Merge", overlay="{}")
    wire(training, "measurement_record", "record.out", "recorded_reading", "base")
    wire(training, "recorded_vocabulary", "out", "recorded_reading", "overlays_0")
    disconnect(training, "learning_replay", "index.append")
    wire(training, "recorded_reading", "out", "learning_replay", "index.append_0")
    # Fragment walking reads the causal token observation stored at that cursor,
    # never the map's newer state from the end of archive traversal.
    extract(training, "historical_tokens", "learning_replay", "observation.json", "measured.tokens", "texts")
    extract(training, "historical_token_json", "learning_replay", "observation.json", "measured.tokens")
    extract(training, "historical_vocabulary", "learning_replay", "observation.json", "measured.vocabulary")
    outputs = training["nodes"]["impulse_map"]["connections"]["outputs"]
    for old_port, replacement, replacement_port in (("hot.hot", "historical_tokens", "texts"),
        ("hot.out", "historical_token_json", "json"),
        ("peak.settled.vocabulary", "historical_vocabulary", "json")):
        retained = []
        for ref in outputs.get(old_port, []):
            if ref["nodeId"] in ("recorded_tokens", "recorded_vocabulary"):
                retained.append(ref)
            else:
                wire(training, replacement, replacement_port, ref["nodeId"], ref["portName"])
        outputs[old_port] = retained
    # The fragment index is ready only after the complete measurement tape.
    for node in training["nodes"].values():
        for port, refs in list(node["connections"]["outputs"].items()):
            if node["id"] == "tape" and port == "finished":
                for ref in refs:
                    wire(training, "measured", "ordered.finished", ref["nodeId"], ref["portName"])
                node["connections"]["outputs"][port] = []
    save("training", training)

# The default measurement archive address is explicit even when this graph is
# launched on its own, not inherited from the raw archive's defaults.
replay = load("measurement_replay")
config = json.loads(load("training")["nodes"]["measured"]["inputData"]["archive.input.through"]["value"])
replay["nodes"]["archive"]["inputData"] = {"input.through": {"value": json.dumps(config, separators=(",", ":"))}}
save("measurement_replay", replay)

for name in ("signals", "system", "capture", "training", "measurement_feed", "measurement_record", "measurement_replay"):
    graph = load(name)
    print(name, len(graph["nodes"]), "nodes")

#!/usr/bin/env python3
"""Finalize the branch migration before generating checked-in schemas."""
from pathlib import Path
import json
import runpy


def replace(path, before, after):
    target = Path(path)
    source = target.read_text()
    if "".join(after.split()) in "".join(source.split()):
        return
    if source.count(before) != 1:
        raise RuntimeError(f"{path}: expected one anchor {before!r}")
    target.write_text(source.replace(before, after, 1))


# The existing miner now accepts either a frame array or one canonical record.
replace("nomagique/temporal/mine.go", "\tvar records []map[string]json.RawMessage\n\n\tif err := json.Unmarshal(envelope.Data, &records); err != nil {\n\t\treturn nil, errnie.Error(errnie.Err(errnie.Validation, \"mine: decode \"+stream.channel+\" data\", err))\n\t}\n", "\trecords, err := mineRecords(envelope.Data)\n\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
# Records are already represented by a shared sequence; no timestamp join.
source = Path("nomagique/temporal/mine.go")
if "func mineRecords(" not in source.read_text():
    with source.open("a") as stream:
        stream.write('''
func mineRecords(encoded json.RawMessage) ([]map[string]json.RawMessage, error) {
    var value any
    if err := json.Unmarshal(encoded, &value); err != nil {
        return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: invalid record data", err))
    }
    if _, array := value.([]any); array {
        var records []map[string]json.RawMessage
        if err := json.Unmarshal(encoded, &records); err != nil {
            return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: invalid record array", err))
        }
        return records, nil
    }
    var record map[string]json.RawMessage
    if err := json.Unmarshal(encoded, &record); err != nil {
        return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: invalid record", err))
    }
    if record == nil {
        return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: null is not a record", nil))
    }
    return []map[string]json.RawMessage{record}, nil
}
''')

# Preserve opaque raw-capture identities even when venue numeric fields are
# projected to Float64. A JSON decoder must not round capture sequences.
replace("nomagique/financial/kraken/futures.go", "\t// Subscription replies, alerts and info name an event; only data frames\n", '''\tvar receipt struct { Capture json.RawMessage `json:"capture"` }
\tif err := json.Unmarshal(payload, &receipt); err != nil {
\t\treturn errnie.Error(errnie.Err(errnie.Validation, "kraken.futures: capture receipt", err))
\t}
\tif len(receipt.Capture) > 0 { frame["capture"] = receipt.Capture }

\t// Subscription replies, alerts and info name an event; only data frames
''')

# A buffered I/O owner must service append and flush signals independently.
# Scalar commit is not a prerequisite for accepting each new payload.
replace("nomagique/store/tables/table.capnp", 'using import "../../runtime/status.capnp".Durable;', 'using import "../../runtime/status.capnp".Durable;\nusing import "../../runtime/status.capnp".Standing;')
replace("nomagique/store/tables/table.capnp", "interface IcebergTable extends(Durable)", "interface IcebergTable extends(Durable, Standing)")

module = runpy.run_path("scripts/wire-boundary-pr.py")
load, add, wire, disconnect, move_output, save, extract = (module[name] for name in
    ("load", "add", "wire", "disconnect", "move_output", "save", "extract"))

feed = load("measurement_feed")
disconnect(feed, "focus", "data")
feed["nodes"]["focus"]["inputData"] = {
    "path": {"value": "data.symbol"}, "operator": {"value": "=="},
    "referencePath": {"value": "_focus"}}
add(feed, "focus_value", "data.Insert", path="_focus", json='"BTC/USD"')
wire(feed, "spot", "out", "focus_value", "data")
wire(feed, "focus_value", "out", "focus", "data")
save("measurement_feed", feed)

for root_name in ("system", "capture"):
    graph = load(root_name)
    if "measurement_event_rows" not in graph["nodes"]:
        disconnect(graph, "measurement_events", "rows")
        add(graph, "measurement_event_rows", "data.Iterate", path="")
        wire(graph, "measurement_mine", "events.out", "measurement_event_rows", "data")
        add(graph, "measurement_event_ready", "data.Filter", path="event_count", operator=">", threshold="0")
        wire(graph, "measurement_event_rows", "out", "measurement_event_ready", "data")
        wire(graph, "measurement_event_ready", "passed", "measurement_archive", "commit")
        add(graph, "measurement_commit_count", "data.Array")
        wire(graph, "measurement_archive", "committed", "measurement_commit_count", "integers_0")
        extract(graph, "measurement_commit", "measurement_commit_count", "out", "0")
        add(graph, "measurement_event_durable", "data.Insert", path="measurement_commit")
        wire(graph, "measurement_event_ready", "out", "measurement_event_durable", "data")
        wire(graph, "measurement_commit", "json", "measurement_event_durable", "json")
        wire(graph, "measurement_event_durable", "out", "measurement_events", "rows_0")
    save(root_name, graph)

training = load("training")
if "both_archives_ready" not in training["nodes"]:
    add(training, "both_archives_ready", "controlflow.Select")
    wire(training, "tape", "finished", "both_archives_ready", "test")
    move_output(training, "walk_ready", "out", "both_archives_ready", "out")
    wire(training, "walk_ready", "out", "both_archives_ready", "yes_0")
save("training", training)

# Report only the wiring relevant to validating this migration.
for name in ("ui_training", "ui_dashboard"):
    graph = load(name)
    printed = 0
    for identifier, node in graph["nodes"].items():
        if node["type"].startswith("ui.") and printed < 8:
            print("UI", name, identifier, json.dumps(node, separators=(",", ":")))
            printed += 1

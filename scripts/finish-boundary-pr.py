#!/usr/bin/env python3
"""Branch migration: write the authored graphs and regenerate their contracts."""
from pathlib import Path
import json


def replace(path, before, after):
    target = Path(path)
    source = target.read_text()
    if after in source:
        return
    if source.count(before) != 1:
        raise RuntimeError(f"{path}: expected one anchor {before!r}")
    target.write_text(source.replace(before, after, 1))


replace("nomagique/data/merge.capnp", "overlay :Data", "overlay :Data, overlays :List(Data)")
replace("nomagique/store/index.capnp", "interface Index {", 'using import "../runtime/status.capnp".Queued;\n\ninterface Index extends(Queued) {')
replace("nomagique/store/index.capnp", "order :Text) -> stream;", "order :Text, unordered :Bool, exhausted :Bool) -> stream;")
replace("nomagique/store/index.capnp", "done @1 () -> (answers :List(Data));", "done @1 () -> Indexed;" )
index_schema = Path("nomagique/store/index.capnp")
if "struct Indexed" not in index_schema.read_text():
    with index_schema.open("a") as stream:
        stream.write('\n# row is emitted only after a pinned archive is exhausted and sorted.\nstruct Indexed {\n  answers @0 :List(Data);\n  pending @1 :UInt64;\n  finished @2 :Bool;\n  union { row @3 :Data; idle @4 :Void; }\n}\n')
replace("nomagique/store/index.go", "\tanswers    [][]byte\n", "\tanswers    [][]byte\n\tunordered bool\n\tsealed bool\n\tordered []indexed\n\trow []byte\n")
replace("nomagique/store/index.go", '\tpartitions := strings.Split(partition, ",")\n', '\tpartitions := strings.Split(partition, ",")\n\n\tif args.Unordered() {\n\t\treturn server.sorted(args, appends, partitions, order)\n\t}\n\n\tif server.unordered {\n\t\treturn server.Error(errnie.Err(errnie.Validation, "index: cannot change archive mode", nil))\n\t}\n')
replace("nomagique/store/index.go", "if len(list) > 0 && compare(coordinate, list[len(list)-1].order) < 0", "if !server.unordered && len(list) > 0 && compare(coordinate, list[len(list)-1].order) < 0")
replace("nomagique/store/index.go", "\tanswers, err := results.NewAnswers(int32(len(server.answers)))", '\tresults.SetIdle()\n\tresults.SetPending(uint64(len(server.ordered)))\n\tresults.SetFinished(server.sealed && len(server.ordered) == 0)\n\n\tif len(server.row) > 0 {\n\t\tif err := results.SetRow(server.row); err != nil {\n\t\t\treturn server.Error(errnie.Err(errnie.Internal, "index: emit ordered row", err))\n\t\t}\n\t}\n\n\tserver.row = nil\n\tanswers, err := results.NewAnswers(int32(len(server.answers)))')

# Array extraction reuses Extract's strict path resolution and existing codec.
replace("nomagique/data/extract.capnp", "unsigned @6 :UInt64;", "unsigned @6 :UInt64; texts @7 :List(Text);")
replace("nomagique/data/extract.go", "\tunsigned uint64\n", "\tunsigned uint64\n\ttexts []string\n")
replace("nomagique/data/extract.go", 'encoding == "json-text" || encoding == "uint64"', 'encoding == "json-text" || encoding == "uint64" || encoding == "texts"')
replace("nomagique/data/extract.go", '\t\tcase "uint64":\n', '\t\tcase "texts":\n\t\t\tvar texts capnp.TextList\n\t\t\ttexts, err = results.NewTexts(int32(len(server.texts)))\n\t\t\tif err == nil {\n\t\t\t\tfor index, text := range server.texts {\n\t\t\t\t\tif err = texts.Set(index, text); err != nil { break }\n\t\t\t\t}\n\t\t\t}\n\t\tcase "uint64":\n')
replace("nomagique/data/extract.go", '"context"\n', '"context"\n\n\tcapnp "capnproto.org/go/capnp/v3"\n')
replace("nomagique/data/extract.go", '\tif server.encoding == "uint64" {\n', '\tif server.encoding == "texts" {\n\t\tarray, valid := value.([]any)\n\t\tif !valid { return errnie.Error(errnie.Err(errnie.Validation, "extract: selected value is not a text array", nil)) }\n\t\tserver.texts = make([]string, len(array))\n\t\tfor index, element := range array {\n\t\t\ttext, valid := element.(string)\n\t\t\tif !valid { return errnie.Error(errnie.Err(errnie.Validation, "extract: array element is not text", nil)) }\n\t\t\tserver.texts[index] = text\n\t\t}\n\t\treturn nil\n\t}\n\n\tif server.encoding == "uint64" {\n')


def node(graph, identifier, kind, **values):
    graph["nodes"][identifier] = {"id": identifier, "type": kind,
        "inputData": {key: {"value": value} for key, value in values.items()},
        "connections": {"inputs": {}, "outputs": {}}}


def wire(graph, source, output, target, port):
    graph["nodes"][source]["connections"]["outputs"].setdefault(output, []).append({"nodeId": target, "portName": port})
    graph["nodes"][target]["connections"]["inputs"].setdefault(port, []).append({"nodeId": source, "portName": output})


def save(name, graph):
    Path("manifest", name + ".json").write_text(json.dumps(graph, indent=2) + "\n")


# This composition is reusable by cold training and the direct replay check.
replay = {"id": "measurement_replay", "name": "Pinned measurement archive to sealed coordinate observations", "nodes": {}}
node(replay, "archive", "definition:archive_scan")
node(replay, "ordered", "store.Index", partition="run", order="sequence", unordered=True)
node(replay, "boundary", "store.Boundary")
wire(replay, "archive", "project.rows", "ordered", "append")
wire(replay, "archive", "scan.exhausted", "ordered", "exhausted")
wire(replay, "ordered", "row", "boundary", "replay")
save("measurement_replay", replay)

for name in ("training_replay", "training_fragment"):
    graph = json.loads(Path("manifest", name + ".json").read_text())
    for identifier, value in graph["nodes"].items():
        if identifier in ("observation", "walked", "fragment_record", "fragment_scope", "with_market", "observation_out", "truth", "ready", "index", "walk_ready"):
            print(name, identifier, json.dumps(value, separators=(",", ":")))

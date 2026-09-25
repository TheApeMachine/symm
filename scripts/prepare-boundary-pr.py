#!/usr/bin/env python3
"""One-time, idempotent source migration for the boundary restoration branch."""
from pathlib import Path
import json


def replace(path, before, after):
    target = Path(path)
    source = target.read_text()
    if after in source:
        return
    if source.count(before) != 1:
        raise RuntimeError(f"{path}: expected exactly one migration anchor: {before!r}")
    target.write_text(source.replace(before, after, 1))


replace("nomagique/data/measurement.capnp",
        "    metadata   @11 :Table(Text, Metadata);",
        "    metadata   @11 :Table(Text, Metadata);\n    producer   @12 :Text;\n    run        @13 :Text;\n    coordinates @14 :List(UInt32);\n    present    @15 :List(Bool);")
replace("nomagique/data/measurement.capnp",
        "        metadata   :Table(Text, Metadata)\n",
        "        metadata   :Table(Text, Metadata),\n        producer   :Text,\n        run        :Text,\n        coordinates :Text,\n        values     :List(Float64),\n        present    :List(Bool)\n")
replace("nomagique/data/measurement.go",
        "\targs := call.Args()\n",
        "\targs := call.Args()\n\tserver.assigned = false\n\tproducer, err := args.Producer()\n\n\tif err != nil {\n\t\treturn measurementProjectionError(\"read producer\", err)\n\t}\n\n\t// Zero is the absent-boundary sentinel, never a measurement epoch.\n\tif producer != \"\" && args.Tick() == 0 {\n\t\treturn nil\n\t}\n")
replace("nomagique/data/measurement.go",
        "\tserver.measurement = measurement\n\tserver.assigned = true",
        "\tif err := server.project(measurement, args); err != nil {\n\t\treturn err\n\t}\n\n\tserver.measurement = measurement\n\tserver.assigned = true")

# A receipt travels with the exact data delivery, never with a later clock.
replace("nomagique/store/grid.capnp",
        "    scope     :List(Text)\n",
        "    scope     :List(Text),\n    run       :Text,\n    sequence  :Int64,\n    receipt   :Data\n")
replace("nomagique/store/grid.capnp",
        "    scope        :Text\n",
        "    scope        :Text,\n    run          :Text,\n    sequence     :Int64,\n    receipt      :Data\n")
replace("nomagique/store/grid.go",
        "\tscope     string\n",
        "\tscope     string\n\trun       string\n\tsequence  int64\n\treceipt   []byte\n")
replace("nomagique/store/grid.go",
        "\tserver.out = nil\n\tserver.values = nil\n",
        "\tserver.run, server.sequence, server.receipt = \"\", 0, nil\n\n\tif feeds.Len() > 0 && call.Args().Sequence() > 0 {\n\t\tif err := server.receiveBoundary(call.Args()); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\n\tserver.out = nil\n\tserver.values = nil\n")
replace("nomagique/store/grid.go",
        "\tresults.SetStatus(runtime.Status(server.Status()))\n",
        "\tresults.SetStatus(runtime.Status(server.Status()))\n\tresults.SetSequence(server.sequence)\n\n\tif err := results.SetRun(server.run); err != nil {\n\t\treturn boundaryError(\"grid: publish run\", err)\n\t}\n\n\tif err := results.SetReceipt(server.receipt); err != nil {\n\t\treturn boundaryError(\"grid: publish receipt\", err)\n\t}\n\n\tserver.run, server.sequence, server.receipt = \"\", 0, nil\n")

# Presence is a delivery fact, not a floating-point sanitization policy.
replace("nomagique/compiler/schema.go",
        "\t\t\tif (rawBits & 0x7ff0000000000000) != 0x7ff0000000000000 {\n\t\t\t\tpresenceList.Set(index, true)\n\t\t\t}",
        "\t\t\tpresenceList.Set(index, true)")

# Diagnostic output is limited to the graph's actual dataflow, not file dumps.
for path in ("manifest/system.json", "manifest/training.json", "manifest/logic.json"):
    graph = json.loads(Path(path).read_text())
    print("GRAPH", path)
    for identifier, node in graph["nodes"].items():
        if path.endswith("system.json") or any(word in identifier for word in ("replay", "signals", "impulse", "reinforce", "paper", "mine", "record")):
            print(identifier, node["type"], "static", node.get("inputData", {}), "inputs", node.get("connections", {}).get("inputs", {}), "outputs", node.get("connections", {}).get("outputs", {}))
print("DATA SCHEMAS", [path.name for path in Path("nomagique/data").glob("*.capnp")])

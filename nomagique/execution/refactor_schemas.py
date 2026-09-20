import os
import glob
import subprocess

schemas = {
    "decide": ["""struct WireDecide {
  eval @0 :AnyPointer;
  minContrast @1 :Float64;
}""", """interface Decide {
  write @0 (decide :WireDecide) -> stream;
  done @1 ();
}"""],
    "gate": ["""struct WireGate {
  action @0 :Text;
}""", """interface Gate {
  write @0 (gate :WireGate) -> stream;
  done @1 ();
}"""],
    "submit": ["""struct WireSubmit {
  action @0 :Text;
  symbol @1 :Text;
}""", """interface Submit {
  write @0 (submit :WireSubmit) -> stream;
  done @1 ();
}"""],
    "regulator": ["""struct WireRegulator {
  payload @0 :AnyPointer;
  symbol @1 :Text;
}""", """interface Regulator {
  write @0 (regulator :WireRegulator) -> stream;
  done @1 ();
}"""]
}

for name, body in schemas.items():
    id_out = subprocess.check_output(["capnpc", "-i"]).decode("utf-8").strip()
    content = f"""using Go = import "/go.capnp";
{id_out};
$Go.package("execution");
$Go.import("nomagique/execution");

{body[0]}

{body[1]}
"""
    with open(f"/Users/theapemachine/go/src/github.com/theapemachine/symm/nomagique/execution/{name}.capnp", "w") as f:
        f.write(content)

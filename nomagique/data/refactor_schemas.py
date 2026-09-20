import os
import glob
import subprocess

schemas = {
    "extract": ["""struct WireExtract {
  payload @0 :AnyPointer;
}""", """interface Extract {
  write @0 (extract :WireExtract) -> stream;
  done @1 ();
}"""],
    "equation": ["""using import "measurement.capnp".WireMeasurement;""", """interface Equation {
  write @0 (measurement :WireMeasurement) -> stream;
  done @1 ();
}"""],
    "select": ["""struct WireSelect {
  payload @0 :AnyPointer;
  path @1 :Text;
}""", """interface Select {
  write @0 (select :WireSelect) -> stream;
  done @1 ();
}"""],
    "series": ["""struct WireSeries {
  payload @0 :AnyPointer;
}""", """interface Series {
  write @0 (series :WireSeries) -> stream;
  done @1 ();
}"""]
}

for name, body in schemas.items():
    id_out = subprocess.check_output(["capnpc", "-i"]).decode("utf-8").strip()
    content = f"""using Go = import "/go.capnp";
{id_out};
$Go.package("data");
$Go.import("nomagique/data");

{body[0]}

{body[1]}
"""
    with open(f"/Users/theapemachine/go/src/github.com/theapemachine/symm/nomagique/data/{name}.capnp", "w") as f:
        f.write(content)

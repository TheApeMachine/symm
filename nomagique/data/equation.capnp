using Go = import "/go.capnp";
@0xc5dfcf2ba5458df1;
$Go.package("data");
$Go.import("nomagique/data");

interface Equation {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}

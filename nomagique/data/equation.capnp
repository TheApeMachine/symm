using Go = import "/go.capnp";
@0xc5dfcf2ba5458df1;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

interface Equation {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}

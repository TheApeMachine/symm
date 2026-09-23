@0xb5523a94f7c3347c;

using Go = import "/go.capnp";
$Go.package("arithmetic");
$Go.import("github.com/theapemachine/symm/nomagique/arithmetic");

# DecimalSubtract takes b from a exactly.
interface DecimalSubtract {
  write @0 (a :Data, b :Data) -> stream;
  done @1 () -> (out :Data);
}

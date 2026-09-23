@0xd8f75d67a4060a9e;

using Go = import "/go.capnp";
$Go.package("arithmetic");
$Go.import("github.com/theapemachine/symm/nomagique/arithmetic");

# DecimalMultiply takes the exact product; it has as many decimal places as
# the operands have together, so nothing is rounded away.
interface DecimalMultiply {
  write @0 (a :Data, b :Data) -> stream;
  done @1 () -> (out :Data);
}

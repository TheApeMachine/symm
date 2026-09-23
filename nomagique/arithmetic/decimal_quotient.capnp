@0xf092db818d202555;

using Go = import "/go.capnp";
$Go.package("arithmetic");
$Go.import("github.com/theapemachine/symm/nomagique/arithmetic");

# DecimalQuotient is a / b rounded down to places decimal places: the most of
# that precision a can pay for at b. A zero divisor is an error.
interface DecimalQuotient {
  write @0 (a :Data, b :Data, places :UInt64) -> stream;
  done @1 () -> (out :Data);
}

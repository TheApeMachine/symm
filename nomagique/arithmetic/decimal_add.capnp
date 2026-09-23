@0xaec506edf4491243;

using Go = import "/go.capnp";
$Go.package("arithmetic");
$Go.import("github.com/theapemachine/symm/nomagique/arithmetic");

# DecimalAdd sums two decimals exactly. Values are JSON numbers, bare or
# quoted, as exchanges write them; the sum keeps every digit either had.
interface DecimalAdd {
  write @0 (a :Data, b :Data) -> stream;
  done @1 () -> (out :Data);
}

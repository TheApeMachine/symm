@0xdef981884b092dc1;
using Go = import "/go.capnp";
$Go.package("paper");
$Go.import("github.com/theapemachine/symm/nomagique/financial/paper");
interface Assessment {
 write @0 (example :Data, predicted :Data, expected :Data, holding :Bool, requiredHolding :Bool) -> stream;
 done @1 () -> (graded :UInt64, correct :UInt64, wrong :UInt64, abstained :UInt64, inapplicable :UInt64);
}

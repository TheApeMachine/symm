@0x8569721ccd586c37;
using Go = import "/go.capnp";
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");
using Record = import "../types/record.capnp".Record;
using Gathered = import "gather.capnp".Gathered;

# Adds this observation's logic coordinates to its causal signal cut. No history
# is retained. Missing logic keeps the combined cut incomplete; signal stamps
# remain untouched and each logic coordinate retains the supplied producer stamp.
interface ExtendCut {
  write @0 (base :Record, values :List(Float64), present :List(Bool), identities :List(Text), epoch :Int64, sequence :Int64) -> stream;
  done @1 () -> Gathered;
}

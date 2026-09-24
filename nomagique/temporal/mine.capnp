@0xe85a3349cc986ec1;
using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

# Mine every record in a JSON channel/data frame using the declared price field.
# State is partitioned by capture session, endpoint, channel, and symbol.
#
# The frames of one session arrive together, in capture order: slot n of
# payload, sequence and endpoint describe one frame, and each is mined as if
# it had arrived alone. When they confirmed a move, events.out holds one
# archive row per confirming frame and events.batch one grading batch per
# stream, carrying its session, endpoint and events.
interface Mine {
 write @0 (payload :List(Data), session :Text, sequence :List(Int64), endpoint :List(Text), channel :Text, priceField :Text) -> stream;
 done @1 () -> Mined;
}
struct Mined {
 union {
  none @0 :Void;
  events :group {
   out @1 :List(Data);
   batch @2 :List(Data);
  }
 }
}

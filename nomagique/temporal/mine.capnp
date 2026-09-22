@0xe85a3349cc986ec1;
using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

# Mine every record in a JSON channel/data frame using the declared price field.
# State is partitioned by capture session, endpoint, channel, and symbol.
interface Mine {
 write @0 (payload :Data, session :Text, sequence :Int64, endpoint :Text, channel :Text, priceField :Text) -> stream;
 done @1 () -> Mined;
}
struct Mined {
 batch @2 :Data;
 union {
  none @0 :Void;
  events :group { out @1 :Data; }
 }
}

@0xfcab90862f056834;
using Go = import "/go.capnp";
$Go.package("kraken");
$Go.import("github.com/theapemachine/symm/nomagique/financial/kraken");
using import "terms.capnp".Terms;

# Authenticated order transport. Admission and position ownership stay with the
# graph's regulator. A failed submit is never automatically retried: its stable
# clientId identifies the request for venue reconciliation.
interface Orders {
 write @0 (publicKey :Data, privateKey :Data, terms :Terms, termsReady :Bool) -> stream;
 done @1 () -> (ready :Bool);
 submit @2 (request :Request) -> (id :Text);
 inspect @3 (id :Text) -> (order :Execution);
 cancel @4 (id :Text) -> (count :UInt32);
 find @5 (clientId :Text) -> (found :Bool, order :Execution);
}
struct Request {
 symbol @0 :Text;
 quantity @1 :Text;
 side @2 :Text;
 clientId @3 :Text;
}
# Cumulative venue facts; quantity, cost, fee and average price are copied
# directly from the SDK response, never recovered from rounded prices.
struct Execution {
 id @0 :Text;
 clientId @1 :Text;
 symbol @2 :Text;
 side @3 :Text;
 status @4 :Text;
 quantity @5 :Text;
 cost @6 :Text;
 fee @7 :Text;
 averagePrice @8 :Text;
}

@0xb25bbcdde0d45d33;
using Go = import "/go.capnp";
$Go.package("kraken");
$Go.import("github.com/theapemachine/symm/nomagique/financial/kraken");

# Venue facts use decimal text; no monetary value crosses Float64.
struct TradingTerms {
 symbol @0 :Text;
 minimumQuantity @1 :Text;
 minimumCost @2 :Text;
 quantityIncrement @3 :Text;
 takerFee @4 :Text;
 costPlaces @5 :UInt32;
}

# Owns the SDK normalizer and authenticated fee lookup. Construction is idle.
# Each quote refreshes account-specific taker fees, while instrument metadata
# is loaded using the SDK's own normalization operation.
interface Terms {
 write @0 (publicKey :Data, privateKey :Data) -> stream;
 done @1 () -> (ready :Bool);
 quote @2 (symbol :Text) -> (terms :TradingTerms);
 normalize @3 (symbol :Text, quantity :Text) -> (symbol :Text, quantity :Text);
 nonce @4 () -> (value :Text);
 balance @5 (symbol :Text) -> (available :Text);
}

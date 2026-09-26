using Go = import "/go.capnp";
@0xa4c158124db502af;
$Go.package("kraken");
$Go.import("github.com/theapemachine/symm/nomagique/financial/kraken");
# Decode the venue instrument frame once; share the same eligible symbols across feeds.
interface Universe {
 write @0 (data :Data, quote :Text, excluded :List(Text)) -> stream;
 done @1 () -> UniverseResult;
}
struct UniverseResult {
 union {
  idle @0 :Void;
  ready :group {
   symbols @1 :List(Text);
   ticker @2 :Data;
   trade @3 :Data;
  }
 }
}

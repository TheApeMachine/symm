using Go = import "/go.capnp";
@0xdcb813faf76d9371;
$Go.package("kraken");
$Go.import("github.com/theapemachine/symm/nomagique/financial/kraken");

# Venue instrument specifications own both subscription eligibility and inbound names.
interface Products {
  write @0 (instruments :List(Data), symbols :List(Text)) -> stream;
  done @1 () -> (subscribe :List(Data), onConnect :List(Data));
  lookup @2 (product :Text) -> (symbol :Text);
}

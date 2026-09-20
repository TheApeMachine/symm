using Go = import "/go.capnp";
@0xedcb06057ba81f9d;
$Go.package("execution");
$Go.import("nomagique/execution");

interface Regulator {
  write @0 (
    id :Text,
    orderId :Text,
    clientOrderId :Text,
    side :Text,
    cumQty :Float64,
    cumCost :Float64,
    fee :Float64,
    status :Text,
    symbol :Text
  ) -> stream;
  done @1 () -> (
    symbol :Text,
    quantity :Float64,
    basis :Float64,
    entryFee :Float64,
    realized :Float64
  );
}

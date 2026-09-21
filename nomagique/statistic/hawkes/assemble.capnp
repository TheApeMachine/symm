using Go = import "/go.capnp";
@0xa4d3b6bbf32f00be;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface Assemble {
  write @0 (data :Data, timestamp :Int64, side :Text, symbol :Text) -> stream;
  done @1 () -> (out :Float64, time :Float64, mark :Float64);
}

@0xc590240cd5e375ed;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Softmax {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}

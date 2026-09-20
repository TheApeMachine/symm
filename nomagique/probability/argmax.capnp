@0xd445d87a71a20851;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Argmax {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}

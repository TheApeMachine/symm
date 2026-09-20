@0xbcddf784fcf09e64;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

struct BackdoorRow {
  values @0 :List(Float64);
}

interface Backdoor {
  write @0 (history :List(BackdoorRow), factualRow :List(Float64)) -> stream;
  done @1 ();
}

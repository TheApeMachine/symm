@0xa4cc327ccd989883;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface Forecast {
  write @0 (in :Float64) -> stream;
  done @1 ();
}

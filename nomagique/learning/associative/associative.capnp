using Go = import "/go.capnp";
@0xc8d7a12b3e4f568a;
$Go.package("associative");
$Go.import("nomagique/learning/associative");

interface Grid {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}

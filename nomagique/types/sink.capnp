@0xd3478951e737bd48;

using Go = import "/go.capnp";
$Go.package("types");
$Go.import("github.com/theapemachine/symm/nomagique/types");

interface Float64Sink {
  write @0 (value :Float64) -> stream;
  done @1 ();
}

interface Int64Sink {
  write @0 (value :Int64) -> stream;
  done @1 ();
}

interface TextSink {
  write @0 (value :Text) -> stream;
  done @1 ();
}

interface BoolSink {
  write @0 (value :Bool) -> stream;
  done @1 ();
}

interface DataSink {
  write @0 (value :Data) -> stream;
  done @1 ();
}

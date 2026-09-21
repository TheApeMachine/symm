using Go = import "/go.capnp";
@0xa8ffe8fde8c3decd;
$Go.package("transport");
$Go.import("nomagique/transport");

interface HTTPRequest {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}

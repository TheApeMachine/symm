using Go = import "/go.capnp";
@0x8f84eb0b54c7b86e;
$Go.package("http");
$Go.import("github.com/theapemachine/symm/nomagique/network/http");

# One HTTP exchange. Method, address, headers and body are graph inputs.
# Each header is one "Name: value" line, gathered from whatever computed it, so
# a signature produced earlier in the graph is a header like any other.
interface HTTPClient {
  write @0 (url :Text, method :Text, headers :List(Data), body :Data) -> stream;
  done @1 () -> (out :Data, status :UInt16);
}

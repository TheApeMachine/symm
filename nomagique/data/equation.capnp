using Go = import "/go.capnp";
@0xc5dfcf2ba5458df1;
$Go.package("data");
$Go.import("nomagique/data");

using import "measurement.capnp".WireMeasurement;

interface Equation {
  write @0 (measurement :WireMeasurement) -> stream;
  done @1 ();
}

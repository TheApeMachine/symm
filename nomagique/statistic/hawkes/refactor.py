import os
import glob
import subprocess

schemas = {
    "assemble": ["""struct WireAssemble {
  payload @0 :AnyPointer;
}""", """interface Assemble {
  write @0 (assemble :WireAssemble) -> stream;
  done @1 ();
}"""],
    "process": ["""struct WireProcess {
  payload @0 :AnyPointer;
}""", """interface Process {
  write @0 (process :WireProcess) -> stream;
  done @1 ();
}"""],
    "event_count": ["""struct WireEventCount {
  payload @0 :AnyPointer;
}""", """interface EventCount {
  write @0 (view :WireEventCount) -> stream;
  done @1 ();
}"""],
    "buy_count": ["""struct WireBuyCount {
  payload @0 :AnyPointer;
}""", """interface BuyCount {
  write @0 (view :WireBuyCount) -> stream;
  done @1 ();
}"""],
    "sell_count": ["""struct WireSellCount {
  payload @0 :AnyPointer;
}""", """interface SellCount {
  write @0 (view :WireSellCount) -> stream;
  done @1 ();
}"""],
    "buy_fraction": ["""struct WireBuyFraction {
  payload @0 :AnyPointer;
}""", """interface BuyFraction {
  write @0 (view :WireBuyFraction) -> stream;
  done @1 ();
}"""],
    "sell_fraction": ["""struct WireSellFraction {
  payload @0 :AnyPointer;
}""", """interface SellFraction {
  write @0 (view :WireSellFraction) -> stream;
  done @1 ();
}"""],
    "arrival_rate": ["""struct WireArrivalRate {
  payload @0 :AnyPointer;
}""", """interface ArrivalRate {
  write @0 (view :WireArrivalRate) -> stream;
  done @1 ();
}"""],
    "buy_rate": ["""struct WireBuyRate {
  payload @0 :AnyPointer;
}""", """interface BuyRate {
  write @0 (view :WireBuyRate) -> stream;
  done @1 ();
}"""],
    "sell_rate": ["""struct WireSellRate {
  payload @0 :AnyPointer;
}""", """interface SellRate {
  write @0 (view :WireSellRate) -> stream;
  done @1 ();
}"""],
    "conditional_intensity": ["""struct WireConditionalIntensity {
  payload @0 :AnyPointer;
}""", """interface ConditionalIntensity {
  write @0 (view :WireConditionalIntensity) -> stream;
  done @1 ();
}"""],
    "buy_intensity": ["""struct WireBuyIntensity {
  payload @0 :AnyPointer;
}""", """interface BuyIntensity {
  write @0 (view :WireBuyIntensity) -> stream;
  done @1 ();
}"""],
    "sell_intensity": ["""struct WireSellIntensity {
  payload @0 :AnyPointer;
}""", """interface SellIntensity {
  write @0 (view :WireSellIntensity) -> stream;
  done @1 ();
}"""],
    "spectral_radius": ["""struct WireSpectralRadius {
  payload @0 :AnyPointer;
}""", """interface SpectralRadius {
  write @0 (view :WireSpectralRadius) -> stream;
  done @1 ();
}"""]
}

for name, body in schemas.items():
    id_out = subprocess.check_output(["capnpc", "-i"]).decode("utf-8").strip()
    content = f"""using Go = import "/go.capnp";
{id_out};
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

{body[0]}

{body[1]}
"""
    with open(f"nomagique/statistic/hawkes/{name}.capnp", "w") as f:
        f.write(content)

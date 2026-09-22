@0x84ab60e8c69728ee;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface Mlr {
    write @0 (
        x :Float64,
        y :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}

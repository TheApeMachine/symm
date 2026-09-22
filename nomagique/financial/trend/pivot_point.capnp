@0xcc5f4ab0fa68f713;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface PivotPoint {
    write @0 (
        open :Float64,
        high :Float64,
        low :Float64,
        close :Float64,
    ) -> stream;

    done @1 () -> (
        p :Float64,
        r1 :Float64,
        r2 :Float64,
        r3 :Float64,
        r4 :Float64,
        s1 :Float64,
        s2 :Float64,
        s3 :Float64,
        s4 :Float64,
    );
}

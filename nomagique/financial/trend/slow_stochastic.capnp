@0xdfa238a84ec5e25e;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface SlowStochastic {
    write @0 (
        value :Float64,
    ) -> stream;

    done @1 () -> (
        k :Float64,
        d :Float64,
    );
}

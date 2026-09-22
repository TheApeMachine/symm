@0xe86799772f8223a7;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface EhlersFisher {
    write @0 (
        high :Float64,
        low :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}

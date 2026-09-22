@0xce2ab44b511b90e5;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface Kst {
    write @0 (
        value :Float64,
    ) -> stream;

    done @1 () -> (
        kst :Float64,
        signal :Float64,
    );
}

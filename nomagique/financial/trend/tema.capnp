@0xaee2d210e639df8a;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface Tema {
    write @0 (
        value :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}

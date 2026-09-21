# Resident-state replay experiment

This command now uses the surviving `sensorium.Manifold` API. The old
`Comparison`, `ProjectionConfig`, and market-frame A/B implementations are not
present in this source tree and are not recreated as compatibility shims.
This is **not** an equivalent replacement for the removed baseline/A/B geometry
comparison, and it makes no claims about correlation significance or denomination
invariance. The old flags and raw-market input format are intentionally rejected.

Input is JSONL, one explicit projector export per line:

```json
{"schema":"sensorium-state-replay/v1","state":{"N":0,"Bytes":[],"Seqs":[],"TokenIDs":[],"ContentIDs":[],"Phase":[],"Omega":[],"Energy":[],"Mass":[],"Heat":[],"Amp":[],"Pos":[],"Vel":[],"Clamped":[],"Dark":[]},"departures":[]}
```

The example documents the tensor schema; it contains no particles and therefore
does **not** pass the exercised-replay check. Real input must contain valid
projected particles. Every one-dimensional tensor has N entries; Pos and Vel have
3*N entries. Repeated ContentIDs refresh residents; departures remove exact IDs.
The caller must export the actual projector state, not synthesize missing fields.

On the supported Metal host, with compiled kernels available:

```sh
go run ./tools/manifoldexperiment -input states.jsonl -output replay.json \
  -grid 32 -steps 100 -repeats 2
```

The report contains the input digest, actual population/readings per advance,
maximum repeat difference, and failures. A failed or empty replay is never labeled
exact replay success. This command does not submit orders or train agent policies.

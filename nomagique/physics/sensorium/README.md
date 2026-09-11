# Sensorium completion integration — 2026-09-11

Read `COMPLETION.md` for the equations, active paths, scope and installation, `DIRECTIVE_STATUS.md` for the audit disposition, and `VALIDATION.md` for executed tests and explicit non-executed acceptance gates.

This is a coordinated source candidate: conservative particle total/auxiliary energy, constrained remapping, gravity mass-flux work, checked space-time pilot paths, reciprocal spectral/material force, configured Hertz hash contacts, scalar-potential/metric GPE, live health viewer and native replay/identification tooling.

**45 new numerical scenarios pass under Clang AddressSanitizer/UndefinedBehaviorSanitizer. Native Metal/CUDA validation and full-application acceptance are not claimed.** No empirical market validation is fabricated.

```sh
python3 tools/install_completion.py /path/to/symm --check
python3 tools/install_completion.py /path/to/symm --apply
```

The root of this bundle is the Sensorium package; `application/` supplies the coordinated changes outside that package. The installer knows both target scopes. Do not copy only the `.metal`/`.cu` files. Restart/rebuild all native and Go consumers together and migrate saved state deliberately.

`patches/` contains review stages against the previous coupled bundle plus the recorded application snapshot. All stages form one integration; they are not independent deployable ABI fragments.

See `tests/run_all.sh` for reproducible CPU tests. The dedicated native scripts execute real code and never substitute CPU protocol doubles.

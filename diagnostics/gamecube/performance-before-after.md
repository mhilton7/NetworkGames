# GameCube performance measurements

## Host request-buffer benchmark

Command:

```sh
go test ./server/nbd-plugin -run '^$' \
  -bench BenchmarkRequestBuffer64KiB -benchmem -count=5
```

Baseline (allocation per NBD read):

```text
8.2–13.7 us/op, 65536 B/op, 1 alloc/op
```

After bounded buffer reuse:

```text
7.795–7.840 ns/op, 0 B/op, 0 allocs/op
```

This microbenchmark measures buffer acquisition only, not TLS, storage, USB,
or game launch. The accepted change removes a 64 KiB allocation from each
64 KiB request after warm-up. Buffers larger than the configured 1 MiB request
limit are not retained.

The runtime path also opens the selected FAT image once, keeps the descriptor
for the session, and uses 64-bit positional `ReadAt`/`WriteAt`; no seek state,
full-image hashing, metadata scan, or synchronous application log occurs in
the read hot path.

## Not yet measured

USB enumeration, Nintendont mount/launch, sequential throughput, random 32/64
KiB median/p95/max latency, Pi CPU/memory/storage utilization, temperature,
undervoltage, reset/stall counts, audio-streaming behavior, and cold/warm
launch comparisons require the deployment and physical-Wii gate. They are
not claimed from host tests.

# End-to-end performance dashboard

The authenticated Performance section models the observable path:

```text
Source → Host backend → NBD/TLS → Pi NBD → USB gadget → Wii
```

It does not claim visibility inside the Wii. USB/Wii-stage throughput and
latency that cannot be measured are labeled unavailable or inferred.
Telemetry failure never changes export readiness or stops serving reads.

## Collection model

The Host uses atomic lifetime counters, gauges, 13 fixed latency buckets
(`<100µs` through `>=500ms`), and a fixed 300-second rate ring. Observations do
not retain paths or payloads, create labels, call the network, or write to disk.
The collectors cover:

- source operations/bytes/latency/errors, handle cache and identity failures;
- metadata/payload/save/zero reads, writes/rejections, extent behavior;
- NBD/TLS connections, requests/bytes/request and TLS-handshake latency,
  queue/errors/reconnects;
- save dirty blocks/bytes, journal, flush/backup/restore/recovery;
- Go heap/target, GC/pause, goroutines, files, CPU estimate, memory limit,
  startup/readiness;
- host TCP retransmission aggregate when `/proc` exposes it.

The authenticated Pi metrics endpoint caches `/proc` and `/sys` sampling for
10 seconds. The Host bridge manager polls it at one configured interval and
shares the result across widgets. It reports firmware uptime, detected board,
CPU/memory/load/temperature, interface/link/Wi-Fi signal, NBD and USB state,
counters, block size, active profile, observed configured-to-unconfigured USB
transitions, and kernel NBD read count/estimated throughput where available.
Short USB transitions between samples may not be observed. No privileged
access or per-widget command is required.

## Sessions and persistence

A session starts after an export is attached and ends on detach, disconnect,
or Host shutdown. The bounded summary includes ID/times, platform/game when
known, Host/firmware/protocol, bytes/reads, average/P95/P99/max latency, source
errors, disconnects, USB resets, save flushes, and outcome. Raw requests are
never persisted. In-memory history is count-bounded (default 100); SQLite
history is pruned by count and configured age (default 30 days).

Current aggregate snapshots are checkpointed at low frequency (default one
minute) to `/data/performance/current.json` using a same-directory synced
temporary file and rename. Session summaries are written only at session end.
No NBD request performs persistent I/O. Persistence failure raises
`PERF-PERSISTENCE-FAILED`; in-memory counters continue.

## APIs and refresh

Authenticated endpoints are:

```text
GET /api/performance/summary
GET /api/performance/host
GET /api/performance/pi
GET /api/performance/session/current
GET /api/performance/sessions?offset=0&limit=50
GET /api/performance/sessions/<id>
GET /api/performance/export?format=json
GET /api/performance/export?format=csv
```

History pages are capped at 100 items and export responses at the retained
history bound. The browser uses one five-second summary refresh and the
manager's cached Pi sample; Pi unavailability leaves Host panels live.

Configuration:

| Variable | Default / valid range |
|---|---|
| `WIIBRIDGE_PERFORMANCE_METRICS_ENABLED` | `true` |
| `WIIBRIDGE_SESSION_HISTORY_ENABLED` | `true` |
| `WIIBRIDGE_MAX_RETAINED_SESSIONS` | `100` (1–1000) |
| `WIIBRIDGE_SESSION_RETENTION_DAYS` | `30` (1–3650) |
| `WIIBRIDGE_PI_METRICS_POLL_INTERVAL` | `10s` (5s–5m) |
| `WIIBRIDGE_DASHBOARD_REFRESH_INTERVAL` | `5s` (2s–30s) |
| `WIIBRIDGE_METRICS_PERSISTENCE_INTERVAL` | `1m` (`0s` or 30s–1h) |
| `WIIBRIDGE_PERF_P99_WARNING` | `100ms` |
| `WIIBRIDGE_PERF_MEMORY_WARNING_PERCENT` | `85` |

Deterministic warnings currently trigger at configured NBD P99, three lifetime
NBD reconnects, save journal above 48 MiB, configured Host memory percentage,
source-cache evictions at least half of ten or more misses, any NBD TLS failure,
or any persistence failure. Codes are `PERF-LATENCY-HIGH`,
`PERF-NBD-RECONNECTS-HIGH`, `PERF-SOURCE-CACHE-THRASHING`,
`PERF-TLS-NEGOTIATION-FAILURES`, `PERF-SAVE-JOURNAL-NEAR-LIMIT`,
`PERF-CONTAINER-MEMORY-HIGH`, and `PERF-PERSISTENCE-FAILED`. Pi memory at 90%,
Pi temperature at 80°C, three USB resets, missing expected Pi telemetry, and
an offline source are added by the integrated Host view. Warnings are signals,
not proof of root cause.

Host benchmark results and measured enabled/disabled overhead are recorded in
`WORKLOG.md` after each release validation. They do not establish Pi Zero W,
USB gadget, Nintendont, or physical-Wii performance.

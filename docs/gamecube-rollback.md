# GameCube rollback

The pre-feature source point is
`64ff84b43700d96c0ba7ab495a006301a9ff2014`. The complete archive is:

```text
[redacted local path]/pre-feature-archive.tar.zst
SHA-256 18c0e7f2897c6f4b3afbc3a9947a4c28fab552f98f7483376849385505b8e332
```

Rollback is operational, not a history rewrite:

1. Detach the USB gadget and disconnect NBD.
2. Preserve any new `.raw` card with `gamecube-saves backup`.
3. Restore the prior TrueNAS immutable image digest:
   `ghcr.io/OWNER/networkgames-host:0.1.0-rc.1-squeakquel-io@sha256:ded61132346b902b8e74e966bb1a97fe5b428a8ffce68d9f9e050faabaf14bbc`.
4. Restore the preserved pre-feature Pi SD card, or its verified image.
5. Keep `/data/gamecube` quarantined but do not delete it until saves are
   recovered. The old host ignores it.
6. Reconnect using the original `connect`/Wii read-only path.
7. Cold launch `SAVE5G` and a second known-good Wii title.

For a separate source checkout without touching this worktree:

```sh
git worktree add ../WiiBridge-wii-rollback \
  64ff84b43700d96c0ba7ab495a006301a9ff2014
```

Do not use `git reset --hard`, delete host datasets, overwrite the working
backup, or replace device credentials as part of rollback.

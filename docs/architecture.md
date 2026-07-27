# Architecture and implementation decision

The host scanner records source device/inode, size, nanosecond mtime, and split
extents. Snapshot construction creates only FAT32 metadata and an extent index.
Every payload read revalidates source identity and opens the source read-only;
no whole payload is read or persisted.

Go was selected for memory safety, bounded concurrency, a mature TLS stack,
small static amd64/ARM binaries, and shared host/Pi maintenance. SQLite uses a
pinned pure-Go driver, WAL transactions, foreign keys, schema migrations, and
atomic active-snapshot publication.

The NBD engine is a standalone fixed-newstyle implementation rather than an
nbdkit plugin. This avoids embedding a scripting runtime and permits the same
memory-safe, static container. It accepts only STARTTLS before export selection,
requires mutual X.509 authentication, bounds option/request sizes and deadlines,
supports read/flush-noop, and rejects write/trim/write-zero. NBD is pinned to
TLS 1.2 because the release host's libnbd 1.22/GnuTLS 3.8.9 combination fails
to select its client certificate for a Go TLS 1.3 CertificateRequest; HTTPS
remains TLS 1.3. Protocol and real parser fuzz tests cover framing, and both
libnbd and the Linux kernel NBD client are exercised independently.

The Pi uses the kernel NBD client with mandatory TLS flags. A typed privileged
helper has six fixed actions. It verifies target board and block read-only state
before ConfigFS attachment. No SMB mount participates in the runtime path.

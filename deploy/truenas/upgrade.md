# Upgrade

1. Snapshot config, data, and backups; export sanitized diagnostics.
2. Record the current immutable image digest and metadata schema.
3. Pull or import the candidate image, verify its digest and SBOM, and replace
   only the image reference in the Compose YAML.
4. Let SIGTERM drain sessions for 30 seconds. Upgrade only with the Wii detached.
5. Start the app and verify health, read-only library mount, active snapshot,
   and metadata persistence. Do not publish a new catalog automatically.

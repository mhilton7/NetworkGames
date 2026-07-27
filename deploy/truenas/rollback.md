# Rollback

Detach the Pi USB gadget and stop NBD sessions. Restore the prior immutable image
digest. If the release notes declare a non-backward-compatible metadata
migration, restore the matching config/data ZFS snapshots. Start the app and
verify the pinned snapshot before reconnecting any Pi.

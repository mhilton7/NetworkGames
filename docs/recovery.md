# Bridge recovery

On backend loss the bridge must remain detached until the configured export,
snapshot identity, and virtual size validate again. A changed identity is never
accepted as a reconnect. The local recovery service exposes diagnostics and
network reprovisioning but does not attach the USB gadget on a wrong board.

For operator recovery:

1. Detach USB through the local UI.
2. Correct network, CA, client certificate, server name, export, or catalog.
3. Run the connection and sample-read checks.
4. Confirm the snapshot ID and size match the intended catalog.
5. Reattach only after the controller reports healthy.

Do not clear identity or reuse a client private key between devices. Sanitized
diagnostics must omit admin tokens and all private-key material.

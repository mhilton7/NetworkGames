# Pinned upstream integration

Inspected 2026-07-26 from the official repositories.

## Pins

- Nintendont: `https://github.com/FIX94/Nintendont`
- Branch: `master`
- Commit: `0f69235b99099ef2851a5f6b7b0e349c572237e8`
- Commit date: 2026-06-02
- USB Loader GX: `https://github.com/wiidev/usbloadergx`
- Commit: `e25c4f3501ed957b7db73f79c51fdf00715ab2e2`
- Version: r1283

## Source conclusions

The pinned Nintendont build uses `NIN_CFG_VERSION 10`. USB Loader GX creates
the standard Nintendont configuration/arguments; this project adds no custom
launcher protocol and does not patch either upstream.

Supported layouts confirmed in source and upstream instructions:

```text
/games/<directory>/game.iso
/games/<directory>/disc2.iso
/games/<directory>/game.gcm
/games/<directory>/game.ciso
/games/<directory>/game.cso
/games/<directory>/sys/boot.bin
```

The uLoader CISO form has a `CISO` header, 2 MiB block size, 0x8000-byte
header, and 1024-entry block map. Nintendont handles disc reads, audio
streaming, video/controller patches, disc switching, and memory-card behavior.

`NIN_CFG_USB` selects USB as Nintendont's root. With that standard path,
emulated cards are under `/saves` on the selected USB root; there is no
verified independent SD save-path override in the inspected interface.
Consequently physical-card mode uses a read-only export, while emulated-card
mode uses a controlled writable GameCube volume. On compatible Wii hardware,
real memory cards remain a Nintendont option. IOS58 and AHB access remain
Wii-side prerequisites managed by the existing loader installation.

USB Loader GX r1283 contains its existing Nintendont integration and writes
the pinned configuration structure. No Wii cIOS setting is changed.

## Packaged official files

The review package copies the files tracked at the pinned Nintendont commit:

| File | SHA-256 |
|---|---|
| `boot.dol` (upstream `loader/loader.dol`) | `e6aaec18db139da234fac29036a3d00f4800ba2bff613b733ababb5dcce9e197` |
| `meta.xml` | `32daae1ee5d0d38b0b5e5bbcdc9485d4c30de8b33164b06cbce6717010aa0008` |
| `icon.png` | `419d3c533dc53cc6c4228d7dd3e273cbf18082c5362e70b061c9af64180b4dc6` |

The upstream repository does not provide a single top-level license file.
Some bundled components provide their own `COPYING` files. This absence is
recorded rather than inventing a license classification; redistribution
review remains an operator/release requirement.

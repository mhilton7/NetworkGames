# Transport isolation

## Decisive-test status

| Test | Evidence | Status |
|---|---|---|
| Original disc | No disc/drive was available to this host | Deferred |
| Conventional Wii USB storage | No operator-controlled USB launch was available | Deferred |
| Alternative container | Current WBFS passes full WIT verification; no legal full ISO was available | Partial |
| Previous firmware/source | `f3a2cb2` retained as known-working differential baseline; no live card overwritten | Pass for source comparison |
| Writable gadget overlay | Historical test accepted five writes and completed 117 NBD requests, but launch still failed | Rejected SCSI write-protection hypothesis |
| Low versus high LBA | Squeakquel below 2 GiB failed identically to high-LBA controls | Rejected 2 GiB-offset hypothesis |
| cIOS bases | 249/base56, 250/base57 and 251/base58 failed identically | Rejected cIOS base as differentiator |
| Live media | `SAVE5G` header, both partitions, final sector and complete hash validated | Pass |
| Live export attribute | `mattrib` reports `A`, not `R` | Pass |

## Layer classification

The failure occurred before game entry and across multiple verified images.
NBD reads completed, the Pi stayed connected, low/high LBAs behaved alike,
and accepting writes did not change the result. USB Loader GX source analysis
showed enumeration uses read access while banner/boot reopens via `O_RDWR`.
The synthesized FAT entries had DOS read-only attributes, precisely matching
“detected but blank banner and return to menu.”

Changing only that FAT attribute produced a physical banner and launch pass
for a verified control while retaining read-only NBD and LUN enforcement.
This isolates the historical launch failure to loader/FAT compatibility, not
media corruption, cIOS, region, WBFS mapping, USB power or network latency.

## Current transport state and missing runtime evidence

The current export has the corrected archive attribute and the same
read-only architecture. The Pi was offline with its SD card attached, so this
task could not collect usbmon, gadget reset/stall counters, undervoltage,
temperature, queue depth, or a synchronized working/failing launch trace.
Those remain mandatory if a post-repair physical Squeakquel launch still
fails.

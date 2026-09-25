# Vaydeer macro keypads — configuration protocol

What this program sends and why. Facts marked **(seen)** were observed on the tested keypad, a
Vaydeer 9-key "Smart Keypad" (model JP-1011, firmware 1.1.5), in September 2026, on Windows 11
and, where it says so, on Linux (Ubuntu 26.04.1); the rest comes
from the open-source projects listed at the end. The 4-key keypad is **not** tested.

## The device

USB `0483:5752`, product string "9-key Smart Keypad", firmware **1.1.5**, bootloader 0.2.2 **(seen)**.
Four USB interfaces **(seen)**:

| Interface | Usage page / usage | Role | Report size |
|---|---|---|---|
| 0 | `FF00` / `01` | command channel (this app talks here) | 65 in, 65 out (incl. report ID 0) |
| 1 | `0001` / `06` | normal keyboard | |
| 2 | `FF00` / `02` | key events (`FB` reports) | 17 in |
| 3 | mouse, consumer (media), system control | | |

Keys are numbered 0–8, left to right, top to bottom: key 0 is top left **(seen: the top-left
key typed 7, and key 0 held VK '7')**.

The number of keys comes from the device info (60, second byte). The 4-key keypad reports
4 there and, according to other projects, uses the same USB ID and protocol with keys 0–3;
its physical key arrangement is not known here **(untested)**. Other key counts are refused.

## Frames

Host → keypad: `[00 report-ID, cmd, len, payload…, xor, zero padding to 65 bytes]`.
Keypad → host: `[cmd, len, status, data…, xor]`, where `len` counts status + data and
`xor` covers cmd, len, status and data **(seen)**. Status 0 = success.
Windows hands back the report ID first; Linux hidraw does not. The HID layer strips it.

Maximum payload is 61 bytes (64 − cmd − len − xor), so a key's data travels in chunks
of 60 bytes after a 1-byte sequence number.

## Commands

| Cmd | Name | Payload | Answer data (after status) |
|---|---|---|---|
| `60` | device info | — | type, keys, fw[3], bl[3], active layer, layer count, max layers **(seen: `01 09 01 01 05 00 02 02 00 01 06`)** |
| `62` | read key | `FF, layer, key` → header; then `seq, layer, key` for seq 0,1,… until data is `FE` | header: `FF, keyType, subType, enc, UTF-16BE label`; chunk: `seq, data…` **(seen)** |
| `67` | read layer name | layer | UTF-16BE name **(seen: "Layer1")** |
| `FD` | init / handshake | — | — (sent before writing, as the hardware-tested tool does) |
| `65` | write layer name | layer, maxLayerIndex, UTF-16BE name | — |
| `61` | write key | header `FF, layer, key, keyType, subType, trigger, label`; chunks `seq, data…` (seq 0..15); end `FE` | — |
| `66` | commit layer | layer, maxLayerIndex | — |
| `FC` | firmware update | **never sent — blocked in `BuildFrame`** | |

`maxLayerIndex` is the highest layer index in use: 0 for a one-layer keypad. This follows
vaydeer-macro-keyboard (used on real hardware). vaydeer-studio-linux sends `max layers − 1`
(5) instead; we do not, because that could tell the keypad 6 layers exist.

Write order (vendor app and hardware-tested tool): `FD`, then per layer `65`, one `61` per key,
`66`. The program reads everything back afterwards and compares. **(seen: a layer with single
keys, Shift combinations, empty keys and a layer name was written on firmware 1.1.5 with
`maxLayerIndex` 0, read back identical, and every key worked in a real application. On Linux
too: a layer name was written and read back on Ubuntu 26.04.1.)**

## Key encodings this app writes

| Kind | keyType | subType | data |
|---|---|---|---|
| empty | 0 | FF | `00` |
| single key | 0 | FF | one Windows virtual-key code **(seen: `37` for "7")** |
| combination | 1 | FF | VK codes, modifiers first, e.g. Shift+F = `10 46` |

Other key types (2 text, 3 program/URL, 4 mouse, 5 macro, 6 Vaydeer, 7 special) are read and
shown but never written: their formats are not verified.

## Layers

The keypad stores up to 6 layers **(seen: 2 layers written and read back)** and
`64 <layer>` makes one active **(seen: works)**. The keypad does **not** switch layers by
itself: Vaydeer's software does it (floating window, mouse wheel, per-application). This app
does it with **layer-switch keys**: an empty key (`keyType 0`, data `00`) whose name is
exactly `Next layer`, `Previous layer` or `Go to layer N`. It types nothing; while the program
runs, it sees the press on interface 2 and sends `64`. The setting lives on the keypad, so any
copy of the program finds it by reading the keypad. **(seen: a layer-switch key in
each of 2 layers switched the keypad back and forth.)**

Key codes: Windows virtual-key codes (A–Z = 65–90, 0–9 = 48–57, F1–F12 = 112–123,
Shift 16, Ctrl 17, Alt 18, Win 91, media 173–179). F13–F24 reportedly send nothing on
this firmware and are not offered.

## Key events (interface 2)

`[FB, 03, layer, key, state, xor, …]`, state 0 = pressed, 2 = released, xor over the five
bytes before it **(seen: the page lit up the right key for each press)**.

On Linux the kernel fetches an interface's reports only while a program has it open. The
tested keypad then behaves as if a key press nobody fetched blocks it **(seen on Ubuntu
26.04.1: before any program ran, the keys typed nothing; a read after such key presses got no
answer to `60`; while `--keepalive` read interface 2, the keys typed and later reads
worked)**. So the app keeps interface 2 open whenever it runs, also while it sends commands
on Linux, and asks `60` once more if the first try gets no answer. `--keepalive` only reads
interface 2.

## Sources

- [alex-savin/go-vaydeer-ninepad-hid](https://github.com/alex-savin/go-vaydeer-ninepad-hid) — protocol notes (MIT)
- [philipp-fischer/vaydeer-macro-keyboard](https://github.com/philipp-fischer/vaydeer-macro-keyboard) — flasher used on hardware; vendor app notes
- [callum-baillie/vaydeer-studio-linux](https://github.com/callum-baillie/vaydeer-studio-linux) — Linux hidraw transport, read-back (MIT)
- [primis.org: Vaydeer 9 Key Linux Fix](https://primis.org/blog/post/2024-06-17/Vaydeer-9-Key-Linux-Fix) — the Linux key-event quirk

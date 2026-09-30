#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

for cmd in go dpkg dpkg-deb; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "Errore: comando richiesto non trovato: $cmd" >&2
        exit 1
    fi
done

UPSTREAM_VERSION="$(
    sed -n 's/^const Version = "\([^"]*\)"/\1/p' internal/app/server.go | head -n1
)"

if [[ -z "$UPSTREAM_VERSION" ]]; then
    echo "Errore: impossibile determinare la versione di ninekeypad." >&2
    exit 1
fi

DESKTOP_REVISION=1
VERSION="${UPSTREAM_VERSION}+desktop${DESKTOP_REVISION}"
ARCH="$(dpkg --print-architecture)"

BUILD_DIR="$ROOT/build"
DEBROOT="$BUILD_DIR/debroot"
DIST_DIR="$ROOT/dist"
DEB="$DIST_DIR/ninekeypad-desktop_${VERSION}_${ARCH}.deb"

echo "==> NineKeypad Desktop ${VERSION} (${ARCH})"

rm -rf "$DEBROOT"

mkdir -p \
    "$BUILD_DIR" \
    "$DIST_DIR" \
    "$DEBROOT/DEBIAN" \
    "$DEBROOT/usr/lib/ninekeypad-desktop/build" \
    "$DEBROOT/usr/lib/ninekeypad-desktop/desktop" \
    "$DEBROOT/usr/share/applications" \
    "$DEBROOT/etc/xdg/autostart" \
    "$DEBROOT/usr/lib/udev/rules.d"

echo "==> Compilazione backend Go"
go build -o "$BUILD_DIR/ninekeypad" ./cmd/ninekeypad

echo "==> Installazione file nel package root"

install -Dm755 \
    "$BUILD_DIR/ninekeypad" \
    "$DEBROOT/usr/lib/ninekeypad-desktop/build/ninekeypad"

install -Dm755 \
    "$ROOT/desktop/ninekeypad_desktop.py" \
    "$DEBROOT/usr/lib/ninekeypad-desktop/desktop/ninekeypad_desktop.py"

cat > "$DEBROOT/usr/share/applications/ninekeypad-desktop.desktop" <<'DESKTOP'
[Desktop Entry]
Type=Application
Name=NineKeypad
Comment=Configure and manage the Vaydeer 9-key Smart Keypad
Exec=/usr/bin/python3 /usr/lib/ninekeypad-desktop/desktop/ninekeypad_desktop.py
Icon=fcitx-unikey
Terminal=false
Categories=Utility;
StartupNotify=true
DESKTOP

cat > "$DEBROOT/etc/xdg/autostart/ninekeypad-desktop.desktop" <<'AUTOSTART'
[Desktop Entry]
Type=Application
Name=NineKeypad
Comment=Start NineKeypad in the system tray
Exec=/usr/bin/python3 /usr/lib/ninekeypad-desktop/desktop/ninekeypad_desktop.py --minimized
Icon=fcitx-unikey
Terminal=false
Categories=Utility;
StartupNotify=false
X-GNOME-Autostart-enabled=true
AUTOSTART

cat > "$DEBROOT/usr/lib/udev/rules.d/70-vaydeer-ninekeypad.rules" <<'UDEV'
SUBSYSTEM=="hidraw", ATTRS{idVendor}=="0483", ATTRS{idProduct}=="5752", TAG+="uaccess"
UDEV

cat > "$DEBROOT/DEBIAN/control" <<EOF_CONTROL
Package: ninekeypad-desktop
Version: $VERSION
Section: utils
Priority: optional
Architecture: $ARCH
Depends: python3, python3-pyside6.qtwebenginewidgets
Maintainer: NineKeypad Desktop Local Build <noreply@localhost>
Description: Desktop integration for the Vaydeer 9-key Smart Keypad
 Native KDE/Qt desktop wrapper for ninekeypad with an embedded
 configuration interface, system tray support, login autostart
 and udev access for the Vaydeer USB 0483:5752 keypad.
EOF_CONTROL

cat > "$DEBROOT/DEBIAN/postinst" <<'POSTINST'
#!/bin/sh
set -e

if command -v udevadm >/dev/null 2>&1; then
    udevadm control --reload-rules || true
    udevadm trigger --subsystem-match=hidraw || true
fi

exit 0
POSTINST

cat > "$DEBROOT/DEBIAN/postrm" <<'POSTRM'
#!/bin/sh
set -e

if command -v udevadm >/dev/null 2>&1; then
    udevadm control --reload-rules || true
fi

exit 0
POSTRM

chmod 755 \
    "$DEBROOT/DEBIAN/postinst" \
    "$DEBROOT/DEBIAN/postrm"

chmod -R go-w "$DEBROOT"

echo "==> Creazione pacchetto Debian"
dpkg-deb --root-owner-group --build "$DEBROOT" "$DEB"

echo
echo "Creato:"
echo "  $DEB"
echo
dpkg-deb --info "$DEB" | sed -n '/ Package:/,$p'

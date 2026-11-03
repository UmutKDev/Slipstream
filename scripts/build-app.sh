#!/usr/bin/env bash
# Slipstream.app paketini derler. macOS'ta Go ve Xcode komut satırı araçları gerekir.
#
# İsteğe bağlı ortam değişkenleri:
#   VERSION         Sürüm numarası (varsayılan 0.1.0)
#   BUNDLE_ID       Paket kimliği (varsayılan com.umutkdev.slipstream)
#   DEVELOPER_ID    İmza kimliği, ör. "Developer ID Application: Ad Soyad (TEAMID)"
#   NOTARY_PROFILE  "xcrun notarytool store-credentials" ile kaydedilmiş profil adı
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-0.1.0}"
BUILD="${BUILD:-$(date +%Y%m%d%H%M)}"
BUNDLE_ID="${BUNDLE_ID:-com.umutkdev.slipstream}"
OUT="$ROOT/build"
APP="$OUT/Slipstream.app"

rm -rf "$OUT" && mkdir -p "$OUT"

echo "› Motor derleniyor (arm64 + x86_64)"
LDFLAGS="-s -w -X github.com/UmutKDev/Slipstream/internal/control.Version=$VERSION"
for arch in arm64 amd64; do
  (cd "$ROOT" && CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -trimpath -ldflags "$LDFLAGS" -o "$OUT/slipstreamd-$arch" ./cmd/slipstreamd)
done
lipo -create -output "$OUT/slipstreamd" "$OUT/slipstreamd-arm64" "$OUT/slipstreamd-amd64"

echo "› Uygulama derleniyor"
# Varsayılan olarak yalnız Apple Silicon. Intel için de derlemek istersen UNIVERSAL=1 ver.
SWIFT_ARCHS=(--arch arm64)
[ "${UNIVERSAL:-0}" = "1" ] && SWIFT_ARCHS=(--arch arm64 --arch x86_64)
(cd "$ROOT/macos" && swift build -c release "${SWIFT_ARCHS[@]}")
BIN="$(cd "$ROOT/macos" && swift build -c release "${SWIFT_ARCHS[@]}" --show-bin-path)/Slipstream"
if [ ! -x "$BIN" ]; then
  echo "✗ Swift çıktısı bulunamadı: $BIN" >&2
  exit 1
fi

echo "› Paket oluşturuluyor"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN" "$APP/Contents/MacOS/Slipstream"
cp "$OUT/slipstreamd" "$APP/Contents/MacOS/slipstreamd"
sed -e "s/__BUNDLE_ID__/$BUNDLE_ID/" -e "s/__VERSION__/$VERSION/" -e "s/__BUILD__/$BUILD/" \
  "$ROOT/macos/Resources/Info.plist" > "$APP/Contents/Info.plist"
[ -f "$ROOT/macos/Resources/AppIcon.icns" ] && cp "$ROOT/macos/Resources/AppIcon.icns" "$APP/Contents/Resources/"

if [ -n "${DEVELOPER_ID:-}" ]; then
  echo "› İmzalanıyor: $DEVELOPER_ID"
  codesign --force --options runtime --timestamp -s "$DEVELOPER_ID" "$APP/Contents/MacOS/slipstreamd"
  codesign --force --options runtime --timestamp -s "$DEVELOPER_ID" "$APP"
  codesign --verify --deep --strict "$APP"
else
  echo "› DEVELOPER_ID yok, geçici imza kullanılıyor (yalnız bu Mac'te çalışır)"
  codesign --force --deep -s - "$APP"
fi

echo "› DMG oluşturuluyor"
DMG="$OUT/Slipstream-$VERSION.dmg"
hdiutil create -volname "Slipstream" -srcfolder "$APP" -ov -format UDZO "$DMG" >/dev/null

if [ -n "${DEVELOPER_ID:-}" ] && [ -n "${NOTARY_PROFILE:-}" ]; then
  echo "› Apple onayına (notarization) gönderiliyor"
  codesign --timestamp -s "$DEVELOPER_ID" "$DMG"
  xcrun notarytool submit "$DMG" --keychain-profile "$NOTARY_PROFILE" --wait
  xcrun stapler staple "$DMG"
fi

echo "✓ Hazır: $APP"
echo "✓ Hazır: $DMG"

#!/usr/bin/env bash
# Slipstream iOS sürümünü hazırlar.
#
#   bash scripts/build-ios.sh            Go çekirdeğini derler, Xcode projesini üretir
#   bash scripts/build-ios.sh --open     ...ve projeyi Xcode'da açar
#   bash scripts/build-ios.sh --install  ...derler ve USB/Wi-Fi ile bağlı iPhone'a kurar
#   bash scripts/build-ios.sh --testflight  ...imzalar ve fastlane ile TestFlight'a yükler
#
# Gerekenler: Go, Xcode, XcodeGen (brew install xcodegen). gomobile yoksa
# betik kendisi kurar. İmza için ekip kimliği anahtar zincirindeki Apple
# sertifikasından bulunur; istersen DEVELOPMENT_TEAM=XXXXXXXXXX ile ver.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
MODE="${1:-}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "hata: $1 bulunamadı. $2" >&2; exit 1; }; }
need go "Homebrew ile kur: brew install go"
need xcodebuild "Xcode'u App Store'dan kur."
need xcodegen "Homebrew ile kur: brew install xcodegen"

export PATH="$(go env GOPATH)/bin:$PATH"
if ! command -v gomobile >/dev/null 2>&1 || ! command -v gobind >/dev/null 2>&1; then
  echo "• gomobile kuruluyor"
  go install golang.org/x/mobile/cmd/gomobile@latest
  go install golang.org/x/mobile/cmd/gobind@latest
fi

echo "• Go çekirdeği iOS için derleniyor (Slipcore.xcframework)"
mkdir -p ios/Frameworks
rm -rf ios/Frameworks/Slipcore.xcframework
gomobile bind -target=ios,iossimulator -iosversion=17.0 -trimpath -ldflags="-s -w" \
  -o ios/Frameworks/Slipcore.xcframework ./mobile

team_from_cert() {
  security find-certificate -c "$1" -p 2>/dev/null \
    | openssl x509 -noout -subject 2>/dev/null \
    | sed -nE 's/.*OU ?= ?([A-Z0-9]{10}).*/\1/p' | head -1
  return 0
}
TEAM="${DEVELOPMENT_TEAM:-}"
[ -n "$TEAM" ] || TEAM="$(team_from_cert "Apple Development" || true)"
[ -n "$TEAM" ] || TEAM="$(team_from_cert "Developer ID Application" || true)"
if [ -n "$TEAM" ]; then
  echo "• Ekip kimliği: $TEAM"
else
  echo "uyarı: ekip kimliği bulunamadı. Xcode'da Signing & Capabilities sekmesinden takımını seç." >&2
fi

echo "• Xcode projesi üretiliyor"
(cd ios && DEVELOPMENT_TEAM="$TEAM" xcodegen generate --quiet)
echo "  ios/Slipstream.xcodeproj hazır"

case "$MODE" in
  --open)
    open ios/Slipstream.xcodeproj
    ;;
  --install)
    echo "• Uygulama derleniyor"
    xcodebuild -project ios/Slipstream.xcodeproj -scheme Slipstream -configuration Release \
      -destination 'generic/platform=iOS' -derivedDataPath build/ios \
      -allowProvisioningUpdates -quiet build
    APP="build/ios/Build/Products/Release-iphoneos/Slipstream.app"
    JSON="$(mktemp)"
    xcrun devicectl list devices --json-output "$JSON" >/dev/null
    DEVICE="$(python3 - "$JSON" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
for dev in d.get("result", {}).get("devices", []):
    hw = dev.get("hardwareProperties", {})
    conn = dev.get("connectionProperties", {})
    if hw.get("platform") == "iOS" and conn.get("tunnelState") != "unavailable":
        print(dev["identifier"]); break
PY
)"
    rm -f "$JSON"
    if [ -z "$DEVICE" ]; then
      echo "hata: bağlı iPhone bulunamadı. Kabloyla bağla, kilidini aç ve \"Bu bilgisayara güven\"i onayla." >&2
      exit 1
    fi
    echo "• iPhone'a kuruluyor ($DEVICE)"
    xcrun devicectl device install app --device "$DEVICE" "$APP"
    xcrun devicectl device process launch --device "$DEVICE" com.umutkdev.slipstream.ios >/dev/null || true
    echo "Kuruldu. Telefonda Slipstream'i aç ve büyük düğmeye dokun."
    ;;
  --testflight)
    need fastlane "Homebrew ile kur: brew install fastlane"
    # App Store Connect API anahtarı: fastlane'in kullandığı .p8 dosyası.
    KEY_PATH="${ASC_KEY_PATH:-$(ls ~/.appstoreconnect/private_keys/AuthKey_*.p8 ~/private_keys/AuthKey_*.p8 2>/dev/null | head -1 || true)}"
    [ -f "$KEY_PATH" ] || { echo "hata: App Store Connect API anahtarı (.p8) bulunamadı. ASC_KEY_PATH ile ver." >&2; exit 1; }
    KEY_ID="${ASC_KEY_ID:-$(basename "$KEY_PATH" .p8 | sed 's/^AuthKey_//')}"
    ISSUER_FILE="$HOME/.appstoreconnect/issuer_id"
    ISSUER="${ASC_ISSUER_ID:-$(cat "$ISSUER_FILE" 2>/dev/null || true)}"
    if [ -z "$ISSUER" ]; then
      read -r -p "Issuer ID (App Store Connect > Kullanıcılar ve Erişim > Entegrasyonlar): " ISSUER
      [ -n "$ISSUER" ] || { echo "hata: Issuer ID gerekli." >&2; exit 1; }
      mkdir -p "$(dirname "$ISSUER_FILE")" && printf '%s\n' "$ISSUER" > "$ISSUER_FILE"
      echo "  Issuer ID $ISSUER_FILE dosyasına kaydedildi, bir dahaki sefere sorulmaz."
    fi
    echo "• TestFlight'a yükleniyor (anahtar $KEY_ID)"
    (cd ios && ASC_KEY_PATH="$KEY_PATH" ASC_KEY_ID="$KEY_ID" ASC_ISSUER_ID="$ISSUER" \
      DEVELOPMENT_TEAM="$TEAM" fastlane beta)
    ;;
esac

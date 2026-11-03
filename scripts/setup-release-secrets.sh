#!/usr/bin/env bash
# Otomatik sürüm iş akışının (.github/workflows/release.yml) ihtiyaç duyduğu
# sırları bu Mac'ten GitHub deposuna yükler. Gizli bilgiler yalnız bu Mac ile
# GitHub arasında taşınır; hiçbir yere yazılmaz ve ekrana basılmaz.
#
# Gerekenler: GitHub CLI (gh auth login yapılmış), anahtar zincirinde
# "Developer ID Application" sertifikası.
#
# Notarization için iki yol var, betik hangisini bulursa onu kullanır:
#   1. App Store Connect API anahtarı (fastlane'in kullandığı .p8 dosyası)
#      ASC_KEY_PATH, ASC_KEY_ID, ASC_ISSUER_ID ortam değişkenleri ya da
#      ~/.appstoreconnect/private_keys/AuthKey_*.p8 ile bulunur.
#   2. Apple kimliği ve uygulamaya özel parola (sorulur).
#
# Kullanım:
#   bash scripts/setup-release-secrets.sh [sertifika.p12]
set -euo pipefail

REPO="${REPO:-UmutKDev/Slipstream}"

say()  { printf '› %s\n' "$*"; }
fail() { printf '✗ %s\n' "$*" >&2; exit 1; }

command -v gh >/dev/null || fail "GitHub CLI yok: brew install gh"
gh auth status >/dev/null 2>&1 || fail "Önce 'gh auth login' ile giriş yap."

put() { # put AD DEĞER  (değer stdin üzerinden gider, komut satırında görünmez)
  printf '%s' "$2" | gh secret set "$1" -R "$REPO" >/dev/null
  say "$1 yüklendi"
}

# --- İmza sertifikası --------------------------------------------------------

IDENTITY=$(security find-identity -v -p codesigning | grep -o '"Developer ID Application: [^"]*"' | head -1 | tr -d '"' || true)
[ -n "$IDENTITY" ] || fail "Anahtar zincirinde 'Developer ID Application' sertifikası bulunamadı. Xcode > Settings > Accounts > Manage Certificates'tan oluştur."
TEAM_ID=$(sed -E 's/.*\(([A-Z0-9]{10})\)$/\1/' <<<"$IDENTITY")
say "İmza kimliği: $IDENTITY"

P12="${1:-}"
if [ -z "$P12" ]; then
  cat <<EOF

Sertifikayı .p12 olarak dışa aktarman gerekiyor (macOS bunu yalnız arayüzden
tek sertifika olarak verir):
  1. Anahtar Zinciri Erişimi açılacak. Soldan "login" ve "Sertifikalarım"ı seç.
  2. "$IDENTITY" satırına sağ tıkla > Dışa Aktar.
  3. Biçim "Kişisel Bilgi Alışverişi (.p12)", bir parola ver ve kaydet.

EOF
  open -a "Keychain Access" 2>/dev/null || true
  read -r -p "Kaydettiğin .p12 dosyasının yolu (sürükleyip bırakabilirsin): " P12
  P12="${P12%\"}"; P12="${P12#\"}"; P12="${P12% }"
fi
[ -f "$P12" ] || fail "Dosya bulunamadı: $P12"
read -r -s -p ".p12 parolası: " P12_PASS; echo
# Parolanın doğru olduğunu yüklemeden önce denetle.
openssl pkcs12 -in "$P12" -passin "pass:$P12_PASS" -nokeys -legacy >/dev/null 2>&1 \
  || openssl pkcs12 -in "$P12" -passin "pass:$P12_PASS" -nokeys >/dev/null 2>&1 \
  || fail ".p12 açılamadı; parola yanlış olabilir."

put MACOS_CERTIFICATE "$(base64 -i "$P12")"
put MACOS_CERTIFICATE_PASSWORD "$P12_PASS"
put APPLE_TEAM_ID "$TEAM_ID"
unset P12_PASS

# --- Notarization ------------------------------------------------------------

KEY_PATH="${ASC_KEY_PATH:-}"
if [ -z "$KEY_PATH" ]; then
  KEY_PATH=$(ls ~/.appstoreconnect/private_keys/AuthKey_*.p8 ~/private_keys/AuthKey_*.p8 2>/dev/null | head -1 || true)
fi

if [ -n "$KEY_PATH" ] && [ -f "$KEY_PATH" ]; then
  KEY_ID="${ASC_KEY_ID:-$(basename "$KEY_PATH" .p8 | sed 's/^AuthKey_//')}"
  ISSUER="${ASC_ISSUER_ID:-}"
  say "App Store Connect API anahtarı bulundu: $KEY_PATH (kimlik $KEY_ID)"
  if [ -z "$ISSUER" ]; then
    read -r -p "Issuer ID (App Store Connect > Kullanıcılar ve Erişim > Entegrasyonlar): " ISSUER
  fi
  [ -n "$ISSUER" ] || fail "Issuer ID gerekli."
  put APPLE_API_KEY "$(base64 -i "$KEY_PATH")"
  put APPLE_API_KEY_ID "$KEY_ID"
  put APPLE_API_ISSUER "$ISSUER"
else
  say "API anahtarı bulunamadı, Apple kimliği ve uygulamaya özel parola kullanılacak."
  DEFAULT_ID=$(git config user.email 2>/dev/null || true)
  read -r -p "Apple kimliği [$DEFAULT_ID]: " APPLE_ID
  APPLE_ID="${APPLE_ID:-$DEFAULT_ID}"
  read -r -s -p "Uygulamaya özel parola (account.apple.com > Oturum açma ve güvenlik): " APP_PASS; echo
  [ -n "$APPLE_ID" ] && [ -n "$APP_PASS" ] || fail "Apple kimliği ve parola gerekli."
  put APPLE_ID "$APPLE_ID"
  put APPLE_APP_PASSWORD "$APP_PASS"
  unset APP_PASS
fi

echo
say "Tamam. Depodaki sırlar:"
gh secret list -R "$REPO"
echo
say "İlk sürüm için: git tag v0.1.0 && git push origin v0.1.0"

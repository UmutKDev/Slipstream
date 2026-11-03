#!/usr/bin/env bash
# TestFlight iş akışının (.github/workflows/testflight.yml) sırlarını
# GitHub deposuna yükler. Mac'te bir kez çalıştır. gh CLI gerekir.
#
# App Store Connect API anahtarı Admin rolünde olmalı: CI'da sertifika
# olmadığı için Xcode, Apple'ın bulutta tuttuğu dağıtım sertifikasıyla
# imzalar ve buna yalnız Admin anahtarı izin verir.
set -euo pipefail

say() { printf '\033[1m%s\033[0m\n' "$*"; }
fail() { echo "hata: $*" >&2; exit 1; }
command -v gh >/dev/null || fail "gh bulunamadı: brew install gh && gh auth login"

REPO="${REPO:-UmutKDev/Slipstream}"
KEY_PATH="${ASC_KEY_PATH:-$(ls ~/.appstoreconnect/private_keys/AuthKey_*.p8 ~/private_keys/AuthKey_*.p8 \
  ~/.private_keys/AuthKey_*.p8 ~/Downloads/AuthKey_*.p8 2>/dev/null | head -1 || true)}"
[ -f "$KEY_PATH" ] || fail "API anahtarı (.p8) bulunamadı. ASC_KEY_PATH ile ver."
KEY_ID="${ASC_KEY_ID:-$(basename "$KEY_PATH" .p8 | sed 's/^AuthKey_//')}"
ISSUER="${ASC_ISSUER_ID:-$(cat ~/.appstoreconnect/issuer_id 2>/dev/null || true)}"
[ -n "$ISSUER" ] || read -r -p "Issuer ID: " ISSUER
[ -n "$ISSUER" ] || fail "Issuer ID gerekli."

TEAM="${DEVELOPMENT_TEAM:-$(security find-certificate -c "Apple Development" -p 2>/dev/null \
  | openssl x509 -noout -subject 2>/dev/null | sed -nE 's/.*OU ?= ?([A-Z0-9]{10}).*/\1/p' | head -1 || true)}"
[ -n "$TEAM" ] || read -r -p "Ekip kimliği (10 karakter): " TEAM

say "Anahtar $KEY_ID, ekip $TEAM, depo $REPO"
base64 -i "$KEY_PATH" | gh secret set APPLE_API_KEY -R "$REPO"
gh secret set APPLE_API_KEY_ID -R "$REPO" -b "$KEY_ID"
gh secret set APPLE_API_ISSUER -R "$REPO" -b "$ISSUER"
gh secret set APPLE_TEAM_ID -R "$REPO" -b "$TEAM"
say "Tamam. TestFlight'a göndermek için: git tag ios-v1.0.1 && git push origin ios-v1.0.1"
say "ya da GitHub'da Actions > testflight > Run workflow."

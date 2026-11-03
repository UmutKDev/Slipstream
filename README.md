# Slipstream

Slipstream, macOS için bir DPI atlatma aracıdır. Türkiye'deki internet servis
sağlayıcılarının SNI filtrelemesini ve DNS zehirlemesini aşmak için tasarlandı.
Menü çubuğunda çalışır, tek tıkla açılıp kapanır.

Slipstream bir VPN değildir. Trafiğini başka bir ülkeye taşımaz, IP adresini
gizlemez. Bağlantılar doğrudan hedef sunucuya gider. Slipstream yalnız ISS'nin DPI
(derin paket incelemesi) cihazının engelli siteyi tanımasını zorlaştırır. Bu
yüzden hız kaybı neredeyse yoktur.

## Nasıl çalışır

Türkiye'deki ISS'ler engelli sitelere erişimi üç yolla keser:

| Yöntem | Ne yapar | Slipstream ne yapar |
| --- | --- | --- |
| DNS zehirleme | Engelli adlar için sahte IP döndürür | Adları şifreli DNS (DoH) ile çözer |
| SNI filtreleme | HTTPS bağlantısının ilk paketindeki site adını okur ve bağlantıyı keser | İlk paketi (ClientHello) site adının ortasından böler |
| HTTP Host filtreleme | Şifresiz isteklerdeki Host başlığını okur, uyarı sayfasına yönlendirir | Host başlığının yazımını değiştirir ve isteği böler |

SNI atlatması için üç teknik birlikte ya da ayrı kullanılır:

- **TLS kaydı bölme:** ClientHello iki ayrı TLS kaydına bölünür. TLS standardı
  buna izin verir, ama çoğu DPI cihazı yalnız ilk kayda bakar.
- **TCP bölme:** İlk paket birden çok TCP segmentine ayrılır. Site adı hiçbir
  segmentte bütün halde görünmez.
- **Gecikme:** Parçalar arasına birkaç milisaniye eklenir. Paketleri birleştiren
  cihazlar zaman aşımına uğrar.

## Profiller

| Profil | Strateji |
| --- | --- |
| Dengeli (önerilen) | Kayıt bölme ve SNI'den TCP bölme |
| Türk Telekom | Kayıt bölme, SNI'den bölme, 2 ms gecikme |
| Turkcell Superonline | Kayıt bölme, 8 baytlık parçalar |
| Vodafone Net | Kayıt bölme, ilk baytı ayrı gönderme, 5 ms gecikme |
| TurkNet | Yalnız SNI'den TCP bölme |
| Millenicom | Kayıt bölme, 16 baytlık parçalar |
| Agresif | 2 baytlık parçalar ve 10 ms gecikme |

ISS kuralları bölgeye ve zamana göre değişir. En güvenilir yol menüdeki
**"En iyi ayarı otomatik bul"** düğmesidir. Slipstream her stratejiyi senin hattında
gerçek sitelerle dener, sertifikayı doğrular ve en hızlı çalışanı seçer.

## Kurulum

### Hazır paketten

`Slipstream-x.y.z.dmg` dosyasını aç ve Slipstream'i Uygulamalar klasörüne sürükle.

### Kaynaktan

Gerekenler: macOS 13 veya üstü, Go 1.24+ ve Xcode komut satırı araçları.

```sh
make test          # motor testleri
make app           # build/Slipstream.app ve build/Slipstream-0.1.0.dmg
```

Developer ID ile imzalamak ve Apple onayından geçirmek için:

```sh
DEVELOPER_ID="Developer ID Application: Ad Soyad (TEAMID)" \
NOTARY_PROFILE="slipstream-notary" \
make app
```

`NOTARY_PROFILE` profilini bir kez şu komutla kaydedersin:
`xcrun notarytool store-credentials slipstream-notary --apple-id ... --team-id ...`

## Sürüm yayınlama

`v0.2.0` gibi bir etiket gönderildiğinde GitHub Actions uygulamayı Developer ID
ile imzalar, Apple'a onaylatır (notarization) ve DMG'yi GitHub Releases'a
yükler:

```sh
git tag v0.2.0 && git push origin v0.2.0
```

Gerekli sırları depoya yüklemek için Mac'te bir kez şunu çalıştır:

```sh
bash scripts/setup-release-secrets.sh
```

Betik anahtar zincirindeki Developer ID sertifikasını ve ekip kimliğini bulur,
sertifikanın .p12 dosyasını ister ve sırları `gh` ile yükler. Notarization
için fastlane'in de kullandığı App Store Connect API anahtarını
(`~/.appstoreconnect/private_keys/AuthKey_*.p8`) kendiliğinden bulur; yoksa
Apple kimliği ve uygulamaya özel parola sorar.

## Komut satırı

Motor (`slipstreamd`) uygulamadan bağımsız da çalışır:

```sh
slipstreamd run -system-proxy        # proxy'yi başlat, sistem proxy'sini ayarla
slipstreamd probe                    # hattında çalışan stratejiyi bul
slipstreamd profiles                 # profilleri listele
slipstreamd sysproxy off             # sistem proxy'sini elle kapat
```

Proxy varsayılan olarak `127.0.0.1:8881` adresinde dinler. Aynı port hem HTTP
hem SOCKS5 kabul eder. Sistem proxy'sini kullanmayan bir uygulamaya bu adresi
elle girebilirsin.

## Mimari

```
Slipstream.app (SwiftUI, menü çubuğu)
  └── slipstreamd (Go motoru)
        ├── proxy     HTTP CONNECT + SOCKS5, tek port
        ├── desync    ilk paketi bölme stratejileri
        ├── tlsx      ClientHello ayrıştırma, SNI konumu
        ├── dns       DoH çözücü, önbellek, yedek sağlayıcılar
        ├── probe     otomatik strateji bulma
        ├── control   127.0.0.1 üzerinde belirteçli kontrol API'si
        └── sysproxy  networksetup ile sistem proxy'si
```

Uygulama her açılışta rastgele bir belirteç üretir ve motoru bu belirteçle
başlatır. Kontrol API'si yalnız bu belirteci taşıyan ve `127.0.0.1` adresine
gelen istekleri kabul eder. Böylece tarayıcıdaki bir web sayfası motoru
yönetemez.

Uygulama beklenmedik biçimde kapanırsa sistem proxy'si ölü bir porta işaret
edebilir. Slipstream bir sonraki açılışta bunu fark eder ve proxy ayarını geri alır.

## Yol haritası

- [x] Go motoru, DoH, profiller, otomatik bulma
- [x] SwiftUI menü çubuğu uygulaması
- [ ] Network Extension: sistem proxy'sine uymayan uygulamalar (ör. Discord
      masaüstü, oyunlar) için tüm trafiği sistem düzeyinde yakalama
- [ ] Discord sesli sohbeti için UDP desteği
- [ ] Otomatik güncelleme (Sparkle)

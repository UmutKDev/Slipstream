# Geçit

Geçit, macOS için bir DPI atlatma aracıdır. Türkiye'deki internet servis
sağlayıcılarının SNI filtrelemesini ve DNS zehirlemesini aşmak için tasarlandı.
Menü çubuğunda çalışır, tek tıkla açılıp kapanır.

Geçit bir VPN değildir. Trafiğini başka bir ülkeye taşımaz, IP adresini
gizlemez. Bağlantılar doğrudan hedef sunucuya gider. Geçit yalnız ISS'nin DPI
(derin paket incelemesi) cihazının engelli siteyi tanımasını zorlaştırır. Bu
yüzden hız kaybı neredeyse yoktur.

## Nasıl çalışır

Türkiye'deki ISS'ler engelli sitelere erişimi üç yolla keser:

| Yöntem | Ne yapar | Geçit ne yapar |
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
**"En iyi ayarı otomatik bul"** düğmesidir. Geçit her stratejiyi senin hattında
gerçek sitelerle dener, sertifikayı doğrular ve en hızlı çalışanı seçer.

## Kurulum

### Hazır paketten

`Gecit-x.y.z.dmg` dosyasını aç ve Geçit'i Uygulamalar klasörüne sürükle.

### Kaynaktan

Gerekenler: macOS 13 veya üstü, Go 1.24+ ve Xcode komut satırı araçları.

```sh
make test          # motor testleri
make app           # build/Gecit.app ve build/Gecit-0.1.0.dmg
```

Developer ID ile imzalamak ve Apple onayından geçirmek için:

```sh
DEVELOPER_ID="Developer ID Application: Ad Soyad (TEAMID)" \
NOTARY_PROFILE="gecit-notary" \
make app
```

`NOTARY_PROFILE` profilini bir kez şu komutla kaydedersin:
`xcrun notarytool store-credentials gecit-notary --apple-id ... --team-id ...`

## Komut satırı

Motor (`gecitd`) uygulamadan bağımsız da çalışır:

```sh
gecitd run -system-proxy        # proxy'yi başlat, sistem proxy'sini ayarla
gecitd probe                    # hattında çalışan stratejiyi bul
gecitd profiles                 # profilleri listele
gecitd sysproxy off             # sistem proxy'sini elle kapat
```

Proxy varsayılan olarak `127.0.0.1:8881` adresinde dinler. Aynı port hem HTTP
hem SOCKS5 kabul eder. Sistem proxy'sini kullanmayan bir uygulamaya bu adresi
elle girebilirsin.

## Mimari

```
Geçit.app (SwiftUI, menü çubuğu)
  └── gecitd (Go motoru)
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
edebilir. Geçit bir sonraki açılışta bunu fark eder ve proxy ayarını geri alır.

## Yol haritası

- [x] Go motoru, DoH, profiller, otomatik bulma
- [x] SwiftUI menü çubuğu uygulaması
- [ ] Network Extension: sistem proxy'sine uymayan uygulamalar (ör. Discord
      masaüstü, oyunlar) için tüm trafiği sistem düzeyinde yakalama
- [ ] Discord sesli sohbeti için UDP desteği
- [ ] Otomatik güncelleme (Sparkle)

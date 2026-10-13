# راهنمای فارسی — sb-ech-gateway

**🇮🇷 فارسی | [🇬🇧 English](README-en.md)**

> کانفیگ‌های **۳x-ui / VLESS + WS + TLS + ECH پشت CDN کلودفلر** را با کلاینت‌های هستهٔ **sing-box**
> (Hiddify، sing-box رسمی، NekoBox، Karing، SFI، GUI.for.SingBox …) سازگار می‌کند — با هستهٔ **Go**
> (تک‌باینری، بدون وابستگی) و یک نصب‌کنندهٔ تعاملی که کل کار را انجام می‌دهد.

---

## ۱) مشکل در یک نگاه

در پنل، مقدار ECH را این‌طور گذاشته‌اید:

```
cloudflare-ech.com+https://8.8.8.8/dns-query
```

این فرمت **مخصوص Xray** است. sing-box مدل دیگری دارد:

| | Xray | sing-box |
|---|---|---|
| پویا | `"echConfigList": "cloudflare-ech.com+https://8.8.8.8/dns-query"` | `"ech": {"enabled": true, "query_server_name": "cloudflare-ech.com"}` |
| استاتیک | base64 در `echConfigList` | `"ech": {"enabled": true, "config": ["-----BEGIN ECH CONFIGS-----", "...","-----END ECH CONFIGS-----"]}` |

و دو نکتهٔ حیاتی (هر دو با تست واقعی تأیید شده):

1. اگر sing-box کلید ECH را پیدا نکند، **کل اتصال fail می‌شود** (`no ECH config found in DNS records`) و به TLS ساده fallback نمی‌کند.
2. کلودفلر کلید مشترک ECH را **فقط روی `cloudflare-ech.com`** منتشر می‌کند؛ دامنهٔ خود شما رکورد `ech=` ندارد. پس ست کردن `query_server_name` **اجباری** است.

ضمناً بررسی سورس پنل ۳x-ui 3.10.0 نشان داد: خروجی sing-box ندارد، برای Clash هم `ech-opts` تولید نمی‌کند و پارسر Hiddify (هستهٔ sing-box) پارامتر `ech=` لینک را نمی‌خواند. به همین دلیل یک گیتوی کوچک بین پنل و کلاینت گذاشته‌ایم.

---

## ۲) نصب (روی سرور، با یک دستور)

```bash
curl -fsSL https://raw.githubusercontent.com/OWNER/sb-ech-gateway/main/scripts/install-remote.sh | sudo bash
```

نصاب تعاملی:

1. خودش پنل را روی `127.0.0.1:2096` پیدا می‌کند و مسیر ساب را می‌پرسد (`/sub` پیش‌فرض؛ در پنل: Settings → Subscription → subPath)
2. یک **secret تصادفی** می‌سازد و در پایان نشان می‌دهد (ذخیره کنید!)
3. باینری Go را می‌سازد/نصب می‌کند (اگر Go نبود، خودش toolchain را دانلود می‌کند) + سرویس systemd
4. به nginx اضافه می‌کند (اگر `nginx -t` خطا بدهد، خودش تغییر را برمی‌گرداند)
5. health check می‌زند و آدرس‌های آمادهٔ ساب را چاپ می‌کند

گزینه‌های نصاب:

```bash
sudo bash install.sh --panel http://127.0.0.1:2096 --sub-path /sub --domain panel.example.com
#   --no-nginx   --no-systemd   --port 2199   --bin-source auto|build|release|local   --yes   --uninstall
```

بعد از نصب:

```bash
sb-ech url        # آدرس‌ها + secret
sb-ech health     # وضعیت ECH و زمان آخرین دریافت کلید
sb-ech logs       # لاگ سرویس
sb-ech edit       # ویرایش config.json و ریستارت
sb-ech update     # به‌روزرسانی از گیت‌هاب
sudo bash /opt/sb-ech-gateway/install.sh --uninstall   # حذف کامل
```

---

## ۳) آدرس‌هایی که به کاربران می‌دهید

```
https://<دامنهٔ پنل>/sb/<SECRET>/auto/<SUBID>    ← ★ یک لینک برای همه (تشخیص با User-Agent)
https://<دامنهٔ پنل>/sb/<SECRET>/sub/<SUBID>     ← sing-box، پویا (query_server_name)
https://<دامنهٔ پنل>/sb/<SECRET>/subs/<SUBID>    ← sing-box، استاتیک PEM (برای Hiddify)
https://<دامنهٔ پنل>/sb/<SECRET>/clash/<SUBID>   ← Clash/Mihomo با ech-opts
https://<دامنهٔ پنل>/sb/<SECRET>/xray/<SUBID>    ← همان لینک‌های خام پنل (v2rayNG و …)
https://<دامنهٔ پنل>/sb/<SECRET>/health          ← وضعیت (برای cron/مانیتورینگ)
```

لینک `/auto/` بر اساس User-Agent تصمیم می‌گیرد (تست‌شده روی نمونهٔ واقعی):

| User-Agent | خروجی |
|---|---|
| Hiddify | sing-box **استاتیک** (چون Hiddify بخش DNS را با تنظیمات خودش جایگزین می‌کند) |
| sing-box / SFI / NekoBox / Karing / GUI.for.SingBox | sing-box **پویا** (کلید تازه در هر اتصال) |
| Clash / Mihomo / Verge / FlClash | YAML با `ech-opts` |
| v2rayNG / Happ / Shadowrocket | همان لینک‌های خام پنل (بدون تغییر) |

می‌توان با کوئری هم دستی انتخاب کرد: `?ech=dynamic` ، `?ech=static` ، `?format=clash` ، `?format=xray`.

---

## ۴) سمت پنل سنایی چه چیزی عوض می‌شود؟ (هیچ)

* کاربران Xray (v2rayNG/Happ/…): همان فیلد ECH Config List با همان مقدار و همان ساب قبلی.
* کاربران sing-box: همین URL جدید را در کلاینت وارد می‌کنند؛ خود گیتوی لینک‌های ساب پنل را می‌خواند.
* اگر بیشتر نودها پشت کلودفلر است، `ech.mode = all` (پیش‌فرض) مناسب است؛ اگر نود غیرکلودفلری دارید `domain-list` بگذارید و دامنه‌ها را در `ech.domains` بنویسید.
* نودهای **XHTTP** در sing-box کار نمی‌کنند (`unknown transport type: xhttp`) و گیتوی آن‌ها را با پیام واضح رد می‌کند؛ برای این کاربران یک نود WS بسازید.

---

## ۵) تست واقعی روی سرور

```bash
/opt/sb-ech-gateway/test-on-server.sh --sub 'http://127.0.0.1:2096/sub/<SUBID>'
/opt/sb-ech-gateway/test-on-server.sh --sub 'http://127.0.0.1:2096/sub/<SUBID>' --ech static
```

این اسکریپت خودش sing-box را دانلود می‌کند، پروفایل را می‌سازد، تونل را بالا می‌آورد و چاپ می‌کند:
IP خروجی بدون پروکسی، IP خروجی از داخل تونل، و خط شاهد ECH:

```
DEBUG dns: exchange cloudflare-ech.com. IN HTTPS
INFO  dns: exchanged HTTPS cloudflare-ech.com. ... ech="AEX+DQBB..."
PASS: tunnel works and traffic exits from your node (203.0.113.7).
```

حداقلِ چیزی که در کانفیگ sing-box لازم است (برای تست دستی):

```jsonc
"tls": {
  "enabled": true,
  "server_name": "دامنهٔ-CDN-شما",
  "alpn": ["h2", "http/1.1"],
  "utls": { "enabled": true, "fingerprint": "chrome" },
  "ech": { "enabled": true, "query_server_name": "cloudflare-ech.com" }
}
```

---

## ۶) شواهد تست محیط واقعی (خلاصه)

| آزمایش | نتیجه |
|---|---|
| ECH پویا با `query_server_name` | ✅ رکورد `HTTPS cloudflare-ech.com ... ech="AEX+DQBB…"` گرفته شد و هندشیک TLS+ECH با CF موفق بود (حتی با uTLS و ALPN=h2) |
| دامنهٔ ECH اشتباه | ❌ `FATAL … no ECH config found in DNS records` (بدون fallback) |
| base64 خام در `ech.config` | ❌ `invalid ECH configs pem` |
| PEM سه‌خطی در `ech.config` | ✅ قبول |
| `xhttp` در sing-box | ❌ `unknown transport type: xhttp` |
| رکورد HTTPS دامنه‌های مشتری CF (`discord.com`, `workers.dev`) | ❌ بدون `ech=` → پس `query_server_name` اجباری است |
| چرخش کلید CF | ✅ مشاهده شد: `config_id` از 64 به 124 در حدود یک ساعت |
| خروجی پنل ۳x-ui 3.10.0 | ❌ بدون sing-box و بدون `ech-opts` در Clash |

لاگ خام: `evidence/ech-tests.log`

---

## ۷) عیب‌یابی

| علامت | علت | راه‌حل |
|---|---|---|
| `no ECH config found in DNS records` | کلاینت رکورد HTTPS دامنهٔ ECH را نگرفت (DNS فیلترشده یا بخش `dns` پروفایل عوض شده) | از `/subs/` (استاتیک) استفاده کنید یا پروفایل `/sub/` را دست‌نخورده نگه دارید |
| در Hiddify پروفایل اضافه نمی‌شود | لینک `vless://` داده‌اید | URL گیتوی (`/auto/` یا `/subs/`) را به‌عنوان پروفایل اضافه کنید |
| قطع و وصل هر چند ساعت با `/subs/` | چرخش کلید CF | بازهٔ آپدیت ساب را ۱ ساعت بگذارید یا از `/sub/` استفاده کنید |
| `unknown transport type: xhttp` | نود XHTTP | نود WS بسازید |
| health = `ok:false` | دسترسی سرور به DoH بسته است | `ech.resolvers` را عوض کنید (مثلاً `https://dns.google/dns-query`) |
| خطای 404 در آدرس‌ها | secret اشتباه | `sb-ech url` |

---

## ۸) امنیت

* `secret` تصادفی ۳۲ کاراکتری، فقط با آن به گیتوی دسترسی هست (secret غلط → 404).
* گیتوی فقط روی `127.0.0.1` گوش می‌دهد؛ سرویس systemd با محدودیت‌های امنیتی اجرا می‌شود.
* `config.json` با مجوز 600 ذخیره می‌شود؛ در لاگ‌ها `subid` ماسک می‌شود.
* برای تست، از یک کلاینت/SUBID موقت استفاده کنید و بعد پاکش کنید.

MIT — ساخته‌شده با تست واقعی روی sing-box v1.14.3 و Cloudflare.

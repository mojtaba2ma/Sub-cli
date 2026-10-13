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
curl -fsSL https://raw.githubusercontent.com/mojtaba2ma/Sub-cli/main/scripts/install-remote.sh | sudo bash
```

نصاب تعاملی (همه چیز را در همان ترمینال می‌پرسد):

1. خودش پنل را روی `127.0.0.1:2096` پیدا می‌کند و مسیر ساب را می‌پرسد (`/sub` پیش‌فرض؛ در پنل: Settings → Subscription → subPath)
2. **دامنهٔ عمومی را می‌پرسد** — این ورودی **پیش‌فرض ندارد و اجباری است**، چون آدرس‌هایی که
   چاپ می‌شوند باید با همان دامنه‌ای ساخته شوند که واقعاً به سرور می‌رسد (هرگز از hostname سرور حدس زده نمی‌شود)
3. **مسیر URL** (پیش‌فرض `/sb/`، Enter = قبول) و **پورت‌ها** را می‌پرسد: پورت محلی سرویس
   (پیش‌فرض `2199`، فقط روی 127.0.0.1) و پورت عمومی در URL (پیش‌فرض `443`؛ اگر پشت کلودفلر
   هستید همان Enter را بزنید) — همه را می‌توانید عوض کنید
4. یک **secret تصادفی** می‌سازد و در پایان نشان می‌دهد (ذخیره کنید!)
5. باینری Go را می‌سازد/نصب می‌کند (اگر Go نبود، خودش toolchain را دانلود می‌کند) + سرویس systemd
6. **به nginx دست نمی‌زند** (پنل nginx شما دست‌نخورده می‌ماند): بلوک آمادهٔ `location <مسیر>/` را
   چاپ می‌کند و در فایل `<prefix>/nginx-location.conf` هم ذخیره می‌کند تا خودتان آن را در
   server block دامنهٔ پنل بگذارید
7. health check می‌زند و آدرس‌های آمادهٔ ساب را چاپ می‌کند (در `<prefix>/urls.txt` هم ذخیره می‌شوند)

گزینه‌های نصاب:

```bash
sudo bash install.sh --panel http://127.0.0.1:2096 --sub-path /sub --domain panel.example.com
#   --path /sb            مسیر URL (پیش‌فرض /sb)
#   --port 2199           پورت محلی سرویس (پیش‌فرض 2199)
#   --public-port 443     پورتی که در URLها می‌آید (پیش‌فرض 443؛ اگر پورت را در URL نمی‌خواهید خالی بگذارید)
#   --new-secret          ساخت secret جدید (پیش‌فرض: secret فعلی حفظ می‌شود تا لینک‌های فعلی کاربران نشکند)
#   --no-systemd   --bin-source auto|build|release|local   --yes   --uninstall
#   --auto-nginx   (اختیاری و پیش‌فرض خاموش: اگر خودتان بخواهید، nginx را خودکار ویرایش می‌کند)
```

> در حالت غیرتعاملی (`--yes` برای اسکریپت/نصب خودکار) **باید `--domain` را بدهید**، وگرنه نصاب
> با پیام خطا خارج می‌شود. مثال: `curl ... | sudo bash -s -- --domain panel.example.com --yes`

### یک قدم دستی: nginx

نصاب **هیچ تغییری در nginx نمی‌دهد**. در پایان، متن آماده را چاپ می‌کند؛ کافی است همان را داخل
`server { ... }` که دامنهٔ پنل را با `listen 443 ssl` سرو می‌کند (کنار locationهای موجودتان) بگذارید:

```nginx
location /sb/ {
    proxy_pass http://127.0.0.1:2199;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 30s;
    proxy_buffering off;
    add_header Cache-Control "no-store" always;
}
```

سپس:

```bash
nginx -t && systemctl reload nginx
```

همین بلوک در `<prefix>/nginx-location.conf` ذخیره شده، پس می‌توانید به‌جای کپی‌کردن فقط این خط را اضافه کنید:

```nginx
include /opt/sb-ech-gateway/nginx-location.conf;
```

> پورت `2199` فقط روی `127.0.0.1` گوش می‌دهد؛ از بیرون فقط از طریق همان دامنه و مسیر `/sb/` قابل دسترسی است.

بعد از نصب:

```bash
sb-ech url        # آدرس‌ها + secret (در <prefix>/urls.txt هم ذخیره می‌شود)
sb-ech health     # وضعیت ECH و زمان آخرین دریافت کلید
sb-ech logs       # لاگ سرویس
sb-ech edit       # ویرایش config.json و ریستارت
sb-ech update     # به‌روزرسانی از گیت‌هاب
sudo bash /opt/sb-ech-gateway/install.sh --uninstall   # حذف کامل
```

---

## ۳) آدرس‌هایی که به کاربران می‌دهید

```
https://<دامنهٔ عمومی که در نصب دادید>/<مسیر>/<SECRET>/auto/<SUBID>   ← ★ یک لینک برای همه (تشخیص با User-Agent؛ مثال با پیش‌فرض‌ها: https://panel.example.com/sb/SECRET/auto/SUBID)
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
* نصاب هیچ فایل nginx را تغییر نمی‌دهد (مگر با `--auto-nginx` که پیش‌فرض خاموش است).
* `config.json` با مجوز 600 ذخیره می‌شود؛ در لاگ‌ها `subid` ماسک می‌شود.
* برای تست، از یک کلاینت/SUBID موقت استفاده کنید و بعد پاکش کنید.

MIT — ساخته‌شده با تست واقعی روی sing-box v1.14.3 و Cloudflare.

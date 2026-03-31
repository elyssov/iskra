## 🔥 Iskra v0.5.0-alpha — Build #17

### ТЕСТЕРАМ: WiFi Direct mesh — нужна ваша помощь!

В этом билде добавлена ключевая фича для митинга: **поиск и связь между телефонами через WiFi Direct** — без интернета, без общей WiFi-сети, без серверов.

#### Что тестировать:

**Тест 1 — WiFi Direct (БЕЗ общей WiFi)**
1. Два телефона с Android 12+ (Samsung, Xiaomi, etc.)
2. Оба **отключают** мобильный интернет
3. Оба **отключают** подключение к WiFi-сети (но WiFi сам должен быть включён!)
4. Оба запускают Искру, вводят PIN
5. Ждут 30-60 секунд
6. Проверяют: появился ли другой телефон в списке контактов или онлайн?
7. Пробуют отправить сообщение

**Тест 2 — Обычная WiFi сеть**
1. Два устройства в одной WiFi-сети (телефон + телефон, или телефон + компьютер)
2. Оба запускают Искру
3. Ждут 10-15 секунд
4. Появляются ли в списке онлайн?

**Тест 3 — WiFi + Ethernet (смешанная сеть)**
1. Телефон в WiFi, компьютер подключён кабелем к тому же роутеру
2. Оба запускают Искру
3. Видят ли друг друга?

#### Что смотреть:
- Появляется ли запрос разрешений (местоположение, WiFi)?
- Не вылетает ли приложение?
- Сколько времени до обнаружения?
- Работает ли отправка сообщений после обнаружения?

#### Что ещё нового в Build #17:
- **Аккаунт "Мастер"** — автоматически добавлен в контакты (золотой бейдж DEV). Пишите сюда жалобы и предложения напрямую через Искру!
- **Каналы** — broadcast один-ко-многим (backend готов, UI базовый)
- **UI обновлён** — чище, минималистичнее, без мультяшных анимаций
- **LAN discovery** — теперь работает на всех сетевых интерфейсах
- **Стабильность** — повреждённые файлы не убивают ядро, retry при старте

**Отзывы:** elyssov@gmail.com или прямо через аккаунт "Мастер" в Искре

---
---

### TESTERS: WiFi Direct mesh — we need your help!

This build adds a critical feature for the protest: **phone-to-phone communication via WiFi Direct** — no internet, no shared WiFi network, no servers.

#### What to test:

**Test 1 — WiFi Direct (NO shared WiFi)**
1. Two phones with Android 12+ (Samsung, Xiaomi, etc.)
2. Both **disable** mobile internet
3. Both **disconnect** from WiFi network (but WiFi itself must be ON!)
4. Both open Iskra, enter PIN
5. Wait 30-60 seconds
6. Check: does the other phone appear in contacts or online list?
7. Try sending a message

**Test 2 — Same WiFi network**
1. Two devices on the same WiFi (phone + phone, or phone + computer)
2. Both open Iskra
3. Wait 10-15 seconds
4. Do they appear in the online list?

**Test 3 — WiFi + Ethernet (mixed network)**
1. Phone on WiFi, computer connected via cable to the same router
2. Both open Iskra
3. Can they see each other?

#### What to look for:
- Does the app ask for permissions (location, WiFi)?
- Does the app crash?
- How long until discovery?
- Does messaging work after discovery?

#### What else is new in Build #17:
- **"Master" account** — auto-added to contacts (gold DEV badge). Report bugs and suggestions here directly through Iskra!
- **Channels** — one-to-many broadcast (backend ready, basic UI)
- **UI refreshed** — cleaner, more minimal, no cartoonish animations
- **LAN discovery** — now works on all network interfaces
- **Stability** — corrupted files don't kill the core, retry on startup

**Feedback:** elyssov@gmail.com or directly via the "Master" account in Iskra

---

**Files:**
- `iskra-build17.apk` — Android (25 MB)
- `iskra-build17.exe` — Windows (10 MB)
- `iskra-linux` — Linux binary
- `relay-linux` — relay server

*"A spark will kindle a flame" / "Из искры возгорится пламя"*

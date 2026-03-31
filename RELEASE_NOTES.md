## 🔥 Iskra v0.5.0-alpha (Build #16.2) — P2P зашифрованный мессенджер

Искра — мессенджер без серверов. Работает когда Telegram заблокирован, интернет отключён, а телефон могут изъять.

---

### 🔒 Безопасность при задержании
- **PIN-код на входе** — обязателен, пропустить нельзя. Argon2id хеширование
- **5 неверных попыток = полное уничтожение данных** (перезапись случайными байтами + удаление)
- **Panic mode** — длительное нажатие на логотип, код 159, мгновенное уничтожение всех данных
- **Правдоподобное отрицание** — после уничтожения генерируются фейковые данные. Приложение выглядит чистым
- **Зашифрованное хранилище** — все контакты, сообщения, группы зашифрованы XSalsa20-Poly1305 (ключ из PIN + seed)

### 🔐 Сквозное шифрование
- **X25519 + XSalsa20-Poly1305** — каждое сообщение зашифровано для конкретного получателя
- **Ed25519 подписи** — подлинность каждого сообщения подтверждена криптографически
- **Proof of Work** — защита от спама
- **Эфемерные ключи** — для каждого сообщения генерируется новый ключ обмена
- **Нет метаданных на серверах** — серверов нет вообще, перехватывать нечего

### 📡 Связь без интернета (сценарий площадь)
- **Wi-Fi Direct mesh** — телефоны связываются напрямую, без роутера и интернета
- **LAN discovery** — автообнаружение устройств на ВСЕХ сетевых интерфейсах (WiFi + Ethernet + VPN)
- **TCP mesh transport** — прямые соединения между устройствами
- **Mesh Wave Protocol** — сообщения распространяются экспоненциально по сети устройств
- **Store-and-forward** — если получатель офлайн, сообщение хранится у промежуточных узлов и доставляется позже
- **Bloom filter дедупликация** — сообщения не дублируются в сети

### 🌐 Связь через интернет (когда есть)
- **WebSocket relay** — работает через `wss://iskra-relay.onrender.com/ws`
- **UDP relay с обфускацией** — XOR-обфускация трафика, выглядит как случайные данные
- **Keepalive 25 сек** — relay не засыпает
- **Революционные псевдонимы** — при подключении к relay каждому назначается случайный псевдоним (Че Гевара, Роза Люксембург, Нестор Махно... 60 имён)
- **Click-to-chat** — клик по псевдониму = автодобавление в контакты и открытие чата

### 💬 Мессенджер
- **Личные чаты** — один на один, сквозное шифрование
- **Групповые чаты** — Ctrl+click для выбора участников, создание группы
- **Каналы (NEW)** — broadcast один-ко-многим. Один автор пишет, все подписчики читают. Для координации на митинге
- **Reply-to** — цитирование и ответ на конкретное сообщение
- **Счётчик непрочитанных** — для личных и групповых чатов
- **Превью последнего сообщения** — в списке контактов
- **Сортировка по активности** — последние чаты наверху
- **Локальное переименование контактов** — видно только вам
- **Удаление чатов** — полная очистка истории

### 💾 Надёжность данных
- **Автосохранение каждые 10 секунд** (Android) — данные не пропадут при выгрузке из памяти
- **Сохранение при отправке и получении** — каждое сообщение сразу на диск
- **Корректное сохранение при выходе** — inbox + группы + каналы
- **Устойчивость к повреждениям** — повреждённые файлы бэкапятся, приложение стартует с чистого состояния
- **3 попытки перезапуска** ядра на Android при ошибке

### 🔑 Идентичность
- **24-словная мнемоника** — ваш ключ. Запишите и сохраните
- **Восстановление на любом устройстве** — ввод мнемоники = восстановление личности
- **Base58 User ID** — компактный, удобный для передачи

### 📱 Платформы
- **Android** — нативное приложение (Kotlin + WebView + Go backend)
- **Windows** — портативный EXE, без установки
- **Linux** — бинарник
- **Foreground service** (Android) — работает в фоне, не убивается системой

### 🌍 Интернационализация
- **Русский и английский** — выбор языка при первом запуске
- **150+ переведённых строк** — весь интерфейс

### 📡 Обновление по воздуху (FOTA)
- **Автопроверка обновлений** при запуске
- **Определение по номеру билда** — каждый новый билд предлагает обновиться
- **Скачивание и установка** прямо из приложения
- **Позже** — не надоедает, запоминает отказ для конкретного билда

### 🏗 Архитектура
- **Go backend** — криптография, mesh, хранилище, API
- **WebView UI** — лёгкий, быстрый, кроссплатформенный
- **gomobile** — Go + Android через JNI
- **Telegram-style UI** — привычный интерфейс, светлая тема
- **60 тестов** — identity, crypto, message, store, mesh, security, integration

---

**Файлы / Files:**
- `iskra-build16.2.apk` — Android (24 MB)
- `iskra-build16.2.exe` — Windows (10 MB)
- `iskra-linux` — Linux binary
- `relay-linux` — relay server

---

*«Из искры возгорится пламя»*

---
---

## 🔥 Iskra v0.5.0-alpha (Build #16.2) — P2P Encrypted Messenger (English)

Iskra is a serverless messenger. It works when Telegram is blocked, the internet is shut down, and your phone may be seized.

---

### 🔒 Security Under Arrest
- **Mandatory PIN on launch** — cannot be skipped. Argon2id hashing
- **5 wrong attempts = complete data destruction** (overwrite with random bytes + delete)
- **Panic mode** — long-press logo, code 159, instant destruction of all data
- **Plausible deniability** — after wipe, fake data is generated. The app looks clean
- **Encrypted storage** — all contacts, messages, groups encrypted with XSalsa20-Poly1305 (key derived from PIN + seed)

### 🔐 End-to-End Encryption
- **X25519 + XSalsa20-Poly1305** — every message encrypted for its specific recipient
- **Ed25519 signatures** — every message is cryptographically authenticated
- **Proof of Work** — spam protection
- **Ephemeral keys** — a new exchange key is generated for every message
- **No metadata on servers** — there are no servers at all, nothing to intercept

### 📡 Communication Without Internet (Square Scenario)
- **Wi-Fi Direct mesh** — phones connect directly, no router or internet needed
- **LAN discovery** — automatic device detection on ALL network interfaces (WiFi + Ethernet + VPN)
- **TCP mesh transport** — direct connections between devices
- **Mesh Wave Protocol** — messages propagate exponentially across the device network
- **Store-and-forward** — if recipient is offline, messages are stored on intermediate nodes and delivered later
- **Bloom filter deduplication** — messages are not duplicated in the network

### 🌐 Communication Over Internet (When Available)
- **WebSocket relay** — connects via `wss://iskra-relay.onrender.com/ws`
- **Obfuscated UDP relay** — XOR traffic obfuscation, looks like random data
- **25-second keepalive** — relay stays awake
- **Revolutionary aliases** — each relay connection gets a random alias (Che Guevara, Rosa Luxemburg, Nestor Makhno... 60 names)
- **Click-to-chat** — click an alias to auto-add to contacts and open chat

### 💬 Messenger
- **Direct messages** — one-on-one, end-to-end encrypted
- **Group chats** — Ctrl+click to select participants, create group
- **Channels (NEW)** — one-to-many broadcast. One author writes, all subscribers read. For protest coordination
- **Reply-to** — quote and reply to specific messages
- **Unread counter** — for both direct and group chats
- **Last message preview** — in contact list
- **Activity-based sorting** — most recent chats on top
- **Local contact renaming** — visible only to you
- **Chat deletion** — full history wipe

### 💾 Data Reliability
- **Auto-save every 10 seconds** (Android) — data survives app being killed from memory
- **Save on send and receive** — every message is written to disk immediately
- **Proper save on exit** — inbox + groups

### 🔑 Identity
- **24-word mnemonic** — your key. Write it down and keep it safe
- **Restore on any device** — enter mnemonic = restore identity
- **Base58 User ID** — compact, easy to share

### 📱 Platforms
- **Android** — native app (Kotlin + WebView + Go backend)
- **Windows** — portable EXE, no installation
- **Linux** — binary
- **Foreground service** (Android) — runs in background, not killed by the system

### 🌍 Internationalization
- **Russian and English** — language selection on first launch
- **150+ translated strings** — the entire UI

### 📡 Over-the-Air Updates (FOTA)
- **Auto-check for updates** on launch
- **Detection by build number** — every new build prompts to update
- **Download and install** directly from the app
- **Later** — remembers dismissal per specific build, will not nag

### 🏗 Architecture
- **Go backend** — cryptography, mesh, storage, API
- **WebView UI** — lightweight, fast, cross-platform
- **gomobile** — Go to Android via JNI
- **Telegram-style UI** — familiar interface, light theme
- **60 tests** — identity, crypto, message, store, mesh, security, integration

---

*"A spark will kindle a flame"*

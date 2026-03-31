## 🔥 Iskra v0.5.0-alpha — Build #17.1 "No Tribble At All"

*Named after that Star Trek episode where fuzzy creatures multiplied uncontrollably on the Enterprise. Our messages won't do that anymore.*

---

### Что нового / What's new

#### 📦 Трюм больше не плодит триблов / Hold no longer breeds Tribbles

Сообщения в mesh-сети теперь имеют lifecycle:

| Параметр | Значение | Зачем |
|----------|----------|-------|
| **HopTTL** | 10 | Макс хопов через сеть |
| **ForwardLimit** | 15 | Макс передач с одного устройства |
| **Морг** | 3 часа | Защита от возврата после исчерпания токенов |
| **Kill switch** | 30 дней | Абсолютный TTL от момента создания автором |

**Как это работает:**
1. Боб отправляет сообщение — оно попадает в трюм с 15 токенами
2. Каждая передача другому устройству = -1 токен
3. Токены = 0 → сообщение в морг на 3 часа (bloom filter всё ещё помнит ID = защита от возврата)
4. Через 3 часа — удалено. Через 30 дней от создания — удалено в любом случае

**Mesh propagation now has a lifecycle:**
1. Bob sends a message — it enters the hold with 15 forward tokens
2. Each sync to another device = -1 token
3. Tokens = 0 → message moves to morgue (3 hours, bloom still remembers ID = return protection)
4. After 3 hours — deleted. After 30 days from creation — killed regardless

---

#### 🔥 Также в этом билде / Also in this build

- **WiFi Direct mesh** — телефоны ищут друг друга без WiFi-сети (DNS-SD advertise + discover)
- **Аккаунт "Мастер"** — связь с разработчиком, автоматически в контактах (золотой бейдж)
- **Каналы** — broadcast один-ко-многим (backend + базовый UI)
- **UI refresh** — чище, минималистичнее, Apple-style
- **LAN discovery на всех интерфейсах** — WiFi + Ethernet видят друг друга
- **Фантомы в онлайне починены** — relay пингует клиентов каждые 20 сек

---

#### ТЕСТЕРАМ / FOR TESTERS

**Тест WiFi Direct — см. предыдущий релиз за инструкциями**

**Отзывы:** elyssov@gmail.com или через аккаунт "Мастер" в Искре

---

**Files:**
- `iskra-build17.1.apk` — Android (25 MB)
- `iskra-build17.1.exe` — Windows (10 MB)
- `iskra-linux` — Linux binary
- `relay-linux` — relay server

*"The only tribble is no tribble" / "Из искры возгорится пламя, но не бесконтрольно"*

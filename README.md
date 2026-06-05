# Iskra / Искра 2.0 Build 11 "El Dorado"

**Peer-to-peer encrypted messenger that works when the internet doesn't.**

*"Поехали!" — Yuri Gagarin, April 12, 1961*

Iskra (Russian: *Искра* — "spark") is a censorship-resistant messenger built for environments where centralized infrastructure is compromised, blocked, or shut down. Every device running Iskra is both a client and a relay node — there is no central server to seize or block.

**What's new in Build 11 "El Dorado" — Reply-Capable + Security Hardening:**
- **Protocol v2 — offline-capable replies.** The author's X25519 public key now ships inside every message, signed end-to-end. You can reply the moment you read, without waiting for the sender to come back online.
- **Signature now covers `AuthorX25519`** — closes a CRITICAL MITM-on-reply where a relay could substitute its own X25519 key after b10's wire-format extension.
- **Stored XSS in relay list — closed.** Relay URLs are now rendered via escaped textContent + event delegation, never via `innerHTML` / `onclick` string interpolation.
- **Android deep-link RCE — closed.** `iskra://` links are passed through `JSONObject.quote()` before any `evaluateJavascript` call.
- **Relay federation hardened.** The `/api/federation` POST now requires `wss://` or `https://` scheme with a non-loopback / non-private host, caps the pool at 64 entries, rate-limits per source IP (1/min), and bounds request body size.
- **Channels store gets `SetVaultKey`** — closes a data-loss gap where encrypted channel state became unreachable after restart.
- **Contacts encrypted at rest** — `Contacts.SetVaultKey()` previously silently discarded the key; now honoured like every other store.
- **Inbox re-saved on PIN setup** — vault key set no longer leaves plaintext on disk until next mutation.
- **Goroutine leaks closed** — `cargo.onPeer` discovery callback bounded by a 16-slot semaphore.
- **Obfuscation keystream extended past 8 KB** — the `byte` counter in `generateObfuscationStream` widened to `uint32` (with v1-compatible 1-byte encoding for the first 8 KB); file chunks no longer leak structure under DPI.
- **Telemetry opt-in default restored.** Was true on cold boot in b10 despite the README's opt-in promise; fresh installs now default to false.

**What's new in Build 9 "El Dorado":**
- **Relay federation** — clients connect to multiple relays simultaneously, exchange relay lists during mesh sync. Anyone can run their own relay.
- **Multi-relay pool** — add/remove relays in Settings, auto-verify availability, 90-day TTL for inactive relays
- **Anonymous telemetry** — opt-in device statistics (SHA-256 hashed device ID, platform, model). No personal data, keys, or messages. Opt-out in Settings.
- **Railway deployment** — relay hosted on Railway.app with auto-deploy from GitHub
- **Dual relay** — Railway (primary) + Render (secondary), both active

**Iskra 2.0 features:**
- Three-tab interface: Contacts / Chats / Mail
- Mail system with 60-day TTL for reliable delivery
- Dark theme with auto-switching (Solntse/Inferno)
- Context menu on contacts (message, letter, QR, forward, rename, delete)
- TTL by content type: chat 15 days, mail 60 days, channels 30 days
- Chunked encrypted file transfer (up to 10 MB)
- Settings page with display name, theme, PIN management, relay list
- 121 commits, ~50 numbered builds across v1.x and v2.0 "Vostok" → "El Dorado"
- 60 test functions across 13 test files; 12 200+ lines of Go and Kotlin

---

## Why Iskra exists

Iskra addresses a critical gap in the open-source ecosystem: secure communication when infrastructure itself is compromised or disabled — protests, internet shutdowns, state-level DPI filtering.

- Full **NaCl / libsodium cryptographic stack:** XSalsa20-Poly1305 + X25519 + Ed25519 + Argon2id + SHA-256 PoW + Bloom-filter dedup.
- **Four transport layers with automatic fallback:** LAN multicast, WebSocket relay, DNS tunneling (base32-in-DNS-queries), Wi-Fi Direct (Android P2P). Each closes a specific failure mode of the network around it.
- **Store-and-forward mesh routing** with Bloom-filter deduplication, payload compression (deflate), and content-type-differentiated TTL.
- **Panic mode:** duress PIN wipes all data and replaces the interface with a decoy profile generating plausible mundane conversations about groceries.
- **Steganographic distribution wrapper** (Pixel Classics) — a retro game collection with a headless Iskra node inside, allowing mesh-network expansion through an innocuous game download.
- **Passed independent security audit** (Borix, April 2026 — Build 8 "Endeavour").
- **Cross-platform:** Windows desktop, Linux, Android (WebView wrapper around Go core via gomobile).
- **Relay federation:** Railway primary, Render secondary; anyone can self-host (`relay.exe`, ~6 MB).

The project is small by star count. That's by design: the target users — journalists, activists, protesters in authoritarian environments — don't star repos. They use the tool and stay invisible.

There is no comparable open-source solution that combines P2P mesh networking, DNS tunneling as transport, steganographic distribution, and a panic / decoy system in a single portable binary. Briar covers some of this ground but lacks DNS tunneling, steganography, panic decoy profiles, and desktop support. Iskra fills a gap that matters to the people who need it most — and who are least likely to leave a public trace of using it.

> *"A spark will kindle a flame"*

---

## How It Works

Iskra uses a **store-and-forward mesh** inspired by 18th-century postal ships:

1. Your message is **encrypted end-to-end** on your device
2. It's stored in a local **hold** (encrypted cargo bay)
3. When your device encounters another Iskra node — via LAN, Wi-Fi Direct, or a relay — they **exchange holds**
4. Each node carries encrypted messages for others, delivering them when paths cross
5. Eventually, the message reaches its recipient and they **decrypt it locally**

No node except the intended recipient can read the message. Intermediate nodes are blind couriers.

```
  You ──encrypt──▶ [hold] ──mesh──▶ [hold] ──mesh──▶ [hold] ──decrypt──▶ Recipient
                     │                │                │
                 your phone      stranger's        friend's
                                  phone             phone
```

## Features

- **End-to-end encryption** — XSalsa20-Poly1305 + X25519 key exchange + Ed25519 signatures
- **No central server** — works over LAN, Wi-Fi Direct, WebSocket relay, or DNS tunnel
- **Store-and-forward mesh** — every node carries encrypted messages for the network
- **Offline-first** — messages queue locally and deliver when connectivity returns
- **Proof-of-Work** — anti-spam, no accounts needed
- **PIN protection** — Argon2id hashing, 5 failed attempts = data wipe
- **Panic mode** — instant wipe with decoy data (fake contacts, fake messages about groceries)
- **Encrypted storage** — inbox encrypted at rest with XSalsa20 (VaultKey)
- **Channels** — one-to-many broadcast (like Telegram channels)
- **Group chats** — with reply-to and per-member delivery
- **File transfer** — chunked, encrypted, up to 10 MB
- **FOTA** — over-the-air updates from GitHub Releases
- **Stealth mode** — traffic obfuscation, DNS tunneling, ICMP masking

## Architecture

```
cmd/
  iskra/           — Desktop binary (Go, localhost HTTP + WebView UI)
  iskra-mobile/    — gomobile bindings for Android
  relay/           — WebSocket relay server (stateless forwarder)
  cargo/           — Headless mesh node (silent "clipper" for stealth delivery)

internal/
  crypto/          — XSalsa20-Poly1305, X25519, Ed25519, PoW, obfuscation
  identity/        — Key generation, base58, BIP39 mnemonic (Russian wordlist)
  message/         — Binary protocol: serialize, encrypt, sign, verify
  mesh/            — Transport layer: LAN TCP, WebSocket relay, DNS tunnel, Wi-Fi Direct
  store/           — Bloom filter, hold (store-and-forward), inbox, contacts, groups
  web/             — HTTP API + static UI (HTML/CSS/JS served from embedded filesystem)

android/           — Kotlin WebView wrapper with Wi-Fi Direct mesh support
```

### Transport Layers

| Layer | How | When |
|-------|-----|------|
| **LAN** | Multicast discovery (239.42.42.42:4242) + TCP sync | Devices on same network |
| **Relay** | WebSocket (wss://) | Internet available, NAT traversal |
| **DNS Tunnel** | Base32-encoded messages in DNS queries | HTTP/WS blocked, DNS still works |
| **Wi-Fi Direct** | Android P2P, mDNS service discovery | No network at all — phone-to-phone |

### Message Lifecycle

```
New → [PoW solved] → [Encrypted] → [Signed] → Hold
  → Broadcast to LAN peers
  → Send via relay (or DNS tunnel fallback)
  → Store-and-forward on every sync
  → HopTTL decrements per forward
  → ForwardLimit (15) prevents flooding
  → 3h morgue after exhaustion
  → 30-day kill switch
```

## Quick Start

### Desktop (Windows/Linux)

Download the latest binary from [Releases](https://github.com/elyssov/iskra/releases):

```bash
# Run with default settings (connects to public relay)
./iskra

# Custom port and data directory
./iskra -port 8080 -data ~/.iskra-data

# Restore from mnemonic
./iskra -restore "word1 word2 word3 ... word24"
```

Open `http://localhost:<port>` in your browser. On first launch, you'll get a 24-word mnemonic — **write it down on paper**.

### Android

Download the APK from [Releases](https://github.com/elyssov/iskra/releases) and install. The Go mesh core runs as a background service with a WebView UI.

### Build from Source

**Prerequisites:** Go 1.24+, Android SDK/NDK (for mobile)

```bash
# Desktop binary
go build -o iskra ./cmd/iskra/

# Relay server
go build -o relay ./cmd/relay/

# Android .aar (requires gomobile)
gomobile bind -target=android/arm64,android/arm -androidapi 24 \
  -o android/app/libs/iskra.aar ./cmd/iskra-mobile/

# Android APK (requires Gradle 8.4, JDK 17+)
cd android && gradle assembleRelease
```

### Run Tests

```bash
go test ./internal/...
```

## Security Model

| Layer | Algorithm | Purpose |
|-------|-----------|---------|
| Encryption | XSalsa20-Poly1305 | Message confidentiality + integrity |
| Key exchange | X25519 (Curve25519) | Ephemeral shared secret per message |
| Signatures | Ed25519 | Message authenticity, non-repudiation |
| Key derivation | Argon2id | PIN → encryption key (time=1, mem=64MB) |
| Anti-spam | SHA-256 PoW | 16-bit difficulty, prevents flooding |
| Deduplication | Bloom filter | 1M capacity, 0.1% false positive rate |
| Storage | XSalsa20 (VaultKey) | Inbox encrypted at rest |

**Threat model:** Iskra assumes the network is hostile. All messages are encrypted before leaving the device. The relay server is a blind forwarder — it never sees plaintext. Even if captured, a device's hold contains only encrypted blobs addressed to other people.

**Panic mode:** Long-press the app title (3 seconds) → enter code → all real data is destroyed and replaced with decoy contacts and fake chat history.

## Relay Server

The relay is a minimal WebSocket forwarder. It does not store messages on disk, does not decrypt anything, and assigns random revolutionary aliases to connected peers (Че, Спартак, Робеспьер...).

```bash
# Self-host a relay
go build -o relay ./cmd/relay/
./relay -port 8443

# Or deploy to Render/Fly.io (see render.yaml / fly.toml)
```

## Protocol

Binary message format (230 bytes + payload):

```
[version:1][id:32][recipientID:20][ttl:4][timestamp:8][contentType:1]
[ephemeralPub:32][nonce:24][payloadLen:4][payload:variable]
[authorPub:32][signature:64][powNonce:8]
```

Content types: `0x01` Text, `0x02` DeliveryConfirm, `0x03` GroupText, `0x04` GroupInvite, `0x05` FileChunk, `0x10` ChannelPost.

## Contributing

This is an active project. Issues and pull requests are welcome.

## License

This project is licensed under the [MIT License](LICENSE).

---

*Built with Go, stubbornness, and the belief that communication is a human right.*

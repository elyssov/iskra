package security

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// decoyContact is a fake contact for plausible deniability.
type decoyContact struct {
	Name      string `json:"name"`
	PubKey    string `json:"pubkey"`
	X25519Pub string `json:"x25519_pub"`
	UserID    string `json:"user_id"`
	AddedAt   int64  `json:"added_at"`
	LastSeen  int64  `json:"last_seen"`
}

type decoyMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	FromPub   string `json:"from_pub"`
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
	Outgoing  bool   `json:"outgoing"`
}

// GenerateDecoy creates a plausible-looking fake messenger state after panic wipe.
// New identity, 3-4 contacts with boring small talk, fake hold messages.
func GenerateDecoy(dataDir string) error {
	os.MkdirAll(dataDir, 0700)

	// Generate fresh seed (new identity — old one is gone)
	seed := make([]byte, 32)
	rand.Read(seed)
	os.WriteFile(filepath.Join(dataDir, "seed.key"), seed, 0600)

	// Generate fake contacts
	contacts := generateDecoyContacts()
	contactsJSON, _ := json.MarshalIndent(contacts, "", "  ")
	os.WriteFile(filepath.Join(dataDir, "contacts.json"), contactsJSON, 0600)

	// Generate fake conversations
	inbox := generateDecoyInbox(contacts)
	inboxJSON, _ := json.MarshalIndent(inbox, "", "  ")
	os.WriteFile(filepath.Join(dataDir, "inbox.json"), inboxJSON, 0600)
	os.MkdirAll(filepath.Join(dataDir, "inbox"), 0700)

	// Generate fake hold (random encrypted-looking blobs)
	generateDecoyHold(dataDir)

	// Empty groups (realistic — most people don't have groups)
	groupsData := []byte(`{"groups":[],"messages":{}}`)
	os.WriteFile(filepath.Join(dataDir, "groups.json"), groupsData, 0600)

	return nil
}

func randomBase58(n int) string {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[idx.Int64()]
	}
	return string(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateDecoyContacts() []decoyContact {
	now := time.Now()
	return []decoyContact{
		{
			Name: "Маша", PubKey: randomBase58(44), X25519Pub: randomBase58(44),
			UserID: randomBase58(20), AddedAt: now.Add(-72 * time.Hour).Unix(),
			LastSeen: now.Add(-2 * time.Hour).Unix(),
		},
		{
			Name: "Дима", PubKey: randomBase58(44), X25519Pub: randomBase58(44),
			UserID: randomBase58(20), AddedAt: now.Add(-120 * time.Hour).Unix(),
			LastSeen: now.Add(-5 * time.Hour).Unix(),
		},
		{
			Name: "Алёна", PubKey: randomBase58(44), X25519Pub: randomBase58(44),
			UserID: randomBase58(20), AddedAt: now.Add(-48 * time.Hour).Unix(),
			LastSeen: now.Add(-12 * time.Hour).Unix(),
		},
		{
			Name: "Серёга", PubKey: randomBase58(44), X25519Pub: randomBase58(44),
			UserID: randomBase58(20), AddedAt: now.Add(-200 * time.Hour).Unix(),
			LastSeen: now.Add(-1 * time.Hour).Unix(),
		},
	}
}

func generateDecoyInbox(contacts []decoyContact) map[string][]decoyMessage {
	inbox := make(map[string][]decoyMessage)

	// ---- Маша: погода, выходные, котик ----
	inbox[contacts[0].UserID] = makeChat(contacts[0], mashaChat())
	// ---- Дима: фильмы, сериалы ----
	inbox[contacts[1].UserID] = makeChat(contacts[1], dimaChat())
	// ---- Алёна: готовка, рецепты ----
	inbox[contacts[2].UserID] = makeChat(contacts[2], alyonaChat())
	// ---- Серёга: работа, спорт ----
	inbox[contacts[3].UserID] = makeChat(contacts[3], seregaChat())

	return inbox
}

func makeChat(c decoyContact, lines []chatLine) []decoyMessage {
	msgs := make([]decoyMessage, len(lines))
	now := time.Now()
	// Spread messages over the last 3 days
	baseTime := now.Add(-72 * time.Hour)

	for i, line := range lines {
		gap := time.Duration(i) * (72 * time.Hour / time.Duration(len(lines)))
		ts := baseTime.Add(gap)

		msgs[i] = decoyMessage{
			ID:        randomHex(16),
			From:      c.UserID,
			FromPub:   c.PubKey,
			Text:      line.text,
			Timestamp: ts.Unix(),
			Status:    "delivered",
			Outgoing:  line.out,
		}
	}
	return msgs
}

type chatLine struct {
	text string
	out  bool // true = we sent it
}

func mashaChat() []chatLine {
	return []chatLine{
		{text: "Привет! Как дела?", out: false},
		{text: "Привет, норм) ты как?", out: true},
		{text: "Отлично! Видела прогноз? Завтра дождь обещают", out: false},
		{text: "Опять? Только потеплело вроде", out: true},
		{text: "Ага, +15 было вчера, а завтра +7 и ливень", out: false},
		{text: "Ну классика, март в России", out: true},
		{text: "Ладно зонт возьму)", out: false},
		{text: "Ты на выходных что делаешь?", out: true},
		{text: "Думала может в парк если погода норм", out: false},
		{text: "А если дождь то дома с сериалом", out: false},
		{text: "Какой смотришь?", out: true},
		{text: "Начала «Слово пацана» пересматривать", out: false},
		{text: "О, я тоже первый сезон залпом проглотил", out: true},
		{text: "Жёстко конечно но затягивает", out: false},
		{text: "Кстати Барсик опять на стол залез и скинул кружку", out: false},
		{text: "😄 классика Барсика", out: true},
		{text: "Третья кружка за месяц", out: false},
		{text: "Может купить пластиковые?)", out: true},
		{text: "Я уже думаю серьёзно об этом", out: false},
		{text: "Ладно побежала, на связи!", out: false},
		{text: "Давай, хороших выходных!", out: true},
		{text: "И тебе 💛", out: false},
	}
}

func dimaChat() []chatLine {
	return []chatLine{
		{text: "Бро, смотрел новый Дюну?", out: false},
		{text: "Ещё нет, стоит?", out: true},
		{text: "Вообще огонь. Визуал как всегда космос", out: false},
		{text: "Вильнёв не подводит", out: false},
		{text: "В кино ходил или дома?", out: true},
		{text: "В IMAX, там надо именно на большом экране", out: false},
		{text: "Ну ок запишу на следующие выходные", out: true},
		{text: "Кстати рекомендую ещё «Оппенгеймер» если не смотрел", out: false},
		{text: "Смотрел, 3 часа но не заскучал", out: true},
		{text: "Нолан гений, чё", out: false},
		{text: "Согласен. А из сериалов что сейчас?", out: true},
		{text: "Fallout на Амазоне неплохой", out: false},
		{text: "Серьёзно? По игре же обычно фигня", out: true},
		{text: "Не, реально нормально сделали. Атмосфера передана", out: false},
		{text: "Ну и Last of Us конечно топ", out: false},
		{text: "Педро Паскаль вообще везде сейчас", out: true},
		{text: "Мандалорец, TLOU, ещё что-то", out: true},
		{text: "Талант, что скажешь", out: false},
		{text: "Ладно пойду гляну Fallout, спасибо за наводку", out: true},
		{text: "Давай, потом обсудим!", out: false},
		{text: "Первые 3 серии сразу смотри, дальше раскачивается", out: false},
		{text: "Ок принял 👍", out: true},
	}
}

func alyonaChat() []chatLine {
	return []chatLine{
		{text: "Привет! Помнишь ты рецепт плова скидывала?", out: true},
		{text: "Привет! Да, щас найду", out: false},
		{text: "Рис басмати, морковь, лук, баранина", out: false},
		{text: "Зира обязательно, барбарис если есть", out: false},
		{text: "Главное — не мешать когда рис сверху!", out: false},
		{text: "А пропорции какие? На 4 порции", out: true},
		{text: "500г мяса, 500г риса, 4 морковки, 2 луковицы", out: false},
		{text: "Спасибо! А сколько воды?", out: true},
		{text: "На 2 пальца выше риса, серьёзно так и меряй)", out: false},
		{text: "Ладно доверюсь опыту 😄", out: true},
		{text: "Кстати я вчера шарлотку пекла, такая пышная получилась!", out: false},
		{text: "О, я шарлотку люблю. В чём секрет?", out: true},
		{text: "Белки отдельно взбивать до пиков", out: false},
		{text: "И яблоки кислые брать, не сладкие", out: false},
		{text: "Антоновка идеально", out: false},
		{text: "Запомню. А то у меня вечно блин а не шарлотка", out: true},
		{text: "Ещё тесто аккуратно лопаткой, не миксером", out: false},
		{text: "Окей попробую на выходных", out: true},
		{text: "Фоткай результат!", out: false},
		{text: "Если будет что фоткать 😅", out: true},
		{text: "Получится, я в тебя верю!", out: false},
		{text: "Спасибо за позитив 🙏", out: true},
		{text: "Кстати, ты борщ как варишь? Со свёклой тушить или в бульоне?", out: true},
		{text: "Обязательно отдельно тушить! С лимонным соком, чтоб цвет держался", out: false},
		{text: "О, вот это лайфхак с лимоном!", out: true},
	}
}

func seregaChat() []chatLine {
	return []chatLine{
		{text: "Здарова! Как на работе?", out: false},
		{text: "Да нормально, дедлайн отодвинули на неделю", out: true},
		{text: "Красава, а у нас наоборот ускорили 🙄", out: false},
		{text: "Сочувствую. Чай хоть бесплатный?", out: true},
		{text: "Чай есть, печеньки тоже. Мелочь а приятно", out: false},
		{text: "Кстати, вчера в футбол гоняли после работы", out: false},
		{text: "И как?", out: true},
		{text: "3:2 выиграли! Я два забил", out: false},
		{text: "Ого, не знал что ты Месси", out: true},
		{text: "Ну так, скрытый талант 😄", out: false},
		{text: "Вы где играете?", out: true},
		{text: "На Спортивной, там крытый манеж, 800р/час на всех", out: false},
		{text: "Не дорого. Может как-нибудь приду", out: true},
		{text: "Давай! Нам как раз вратарь нужен", out: false},
		{text: "Я так и знал что подвох будет", out: true},
		{text: "😂😂😂", out: false},
		{text: "Ладно серьёзно, приходи, весело будет", out: false},
		{text: "Ок давай на следующую среду", out: true},
		{text: "Записал! Форму возьми любую", out: false},
		{text: "Бутсы нужны?", out: true},
		{text: "Не, там покрытие искусственное, кроссовки норм", out: false},
		{text: "Понял, договорились 💪", out: true},
		{text: "Кстати видел Спартак вчера проиграл?", out: false},
		{text: "Не смотрю за ними, одно расстройство", out: true},
		{text: "Это точно...", out: false},
	}
}

func generateDecoyHold(dataDir string) {
	holdDir := filepath.Join(dataDir, "hold")
	os.MkdirAll(holdDir, 0700)

	// Generate 150-350 random "encrypted messages" in hold
	nBig, _ := rand.Int(rand.Reader, big.NewInt(200))
	count := 150 + int(nBig.Int64())

	for i := 0; i < count; i++ {
		// Random message ID
		id := make([]byte, 32)
		rand.Read(id)
		filename := fmt.Sprintf("%x.msg", id)

		// Random encrypted-looking payload (100-800 bytes)
		sizeBig, _ := rand.Int(rand.Reader, big.NewInt(700))
		size := 100 + int(sizeBig.Int64())
		payload := make([]byte, size)
		rand.Read(payload)

		os.WriteFile(filepath.Join(holdDir, filename), payload, 0600)
	}
}

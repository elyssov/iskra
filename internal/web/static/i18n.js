// Iskra — Internationalization
// Language: ru (default), en

window._lang = 'ru';

const I18N = {
  ru: {
    // PIN screen
    pin_enter: 'Введите PIN-код',
    pin_setup: 'Установите PIN-код (4-6 цифр)',
    pin_confirm: 'Повторите PIN-код',
    pin_wrong: 'Неверный PIN',
    pin_mismatch: 'PIN-коды не совпадают',
    pin_btn_save: 'Сохранить',
    pin_btn_confirm: 'Подтвердить',
    pin_btn_login: 'Войти',
    pin_btn_change: 'Сменить PIN',
    pin_remaining: 'Осталось попыток:',
    pin_wiped: 'Данные уничтожены',

    // Onboarding
    onb_subtitle: 'Свободный мессенджер без серверов',
    onb_key_created: 'Ваш ключ создан. Это ваш адрес в сети Искра — как номер телефона, только его невозможно заблокировать.',
    onb_copy_link: 'Скопировать визитку для друзей',
    onb_write_words: 'Запишите эти 24 слова',
    onb_words_warning: 'Это единственный способ восстановить доступ. Без них ваши сообщения потеряны навсегда. Запишите на бумагу. Не фотографируйте.',
    onb_start: 'Я записал(а) слова. Начать.',
    onb_restore: 'У меня уже есть 24 слова',

    // Welcome screen
    welcome_title: 'Добро пожаловать в Искру',
    welcome_text: 'Выберите контакт слева или добавьте нового, чтобы начать общение.',
    welcome_encrypted: 'Сквозное шифрование. Никто не может прочитать ваши сообщения.',
    welcome_quote: '«Из искры возгорится пламя»',

    // Sidebar buttons
    btn_add: 'Добавить',
    btn_group: 'Группа',
    btn_key: 'Ключ',

    // Chat
    chat_select: 'Выберите контакт',
    chat_placeholder: 'Написать сообщение...',
    typing: 'печатает...',
    msg_empty: 'Начните разговор — напишите первое сообщение',
    msg_empty_group: 'Групповой чат создан. Напишите первое сообщение!',
    msg_loading: 'Загрузка...',

    // Time
    time_today: 'Сегодня',
    time_yesterday: 'Вчера',

    // Status
    status_relay: 'relay',
    status_nearby: 'рядом',
    status_hold: 'в трюме',

    // Online
    online_now: 'В сети сейчас',
    online_contact: 'контакт',
    online_click: 'Нажмите чтобы написать',

    // Contacts empty
    contacts_empty_title: 'Пока никого нет',
    contacts_empty_text: 'Нажмите «+ Добавить» внизу, чтобы добавить первый контакт. Попросите друга прислать вам свою визитку из Искры.',
    contacts_members: 'участников',

    // Buttons
    btn_cancel: 'Отмена',
    btn_create: 'Создать',
    btn_restore: 'Восстановить',
    btn_copied: 'Скопировано!',
    btn_later: 'Позже',
    btn_update: 'Обновить сейчас',
    btn_close: 'Закрыть',
    btn_ok: 'Понятно',
    btn_restart: 'Перезапустить',

    // Modal: Key
    modal_key_title: '🔑 Ваша визитка',
    modal_key_text: 'Отправьте визитку друзьям любым способом — через Telegram, SMS, email, продиктуйте вслух. По ней они добавят вас в свою Искру.',
    modal_key_copy: 'Скопировать визитку',
    modal_key_tech: 'Технические данные (для продвинутых)',
    modal_key_mnemonic: '24 слова для восстановления',
    modal_key_warning: 'Никому не показывайте эти слова. Тот, кто их знает — может читать все ваши сообщения.',

    // Modal: Add contact
    modal_add_title: 'Добавить контакт',
    modal_add_text: 'Вставьте визитку друга (начинается с iskra://) или введите данные вручную.',
    modal_add_invite_ph: 'Вставьте визитку iskra://...',
    modal_add_or: 'или вручную',
    modal_add_name_ph: 'Имя друга',
    modal_add_pubkey_ph: 'Публичный ключ',
    modal_add_x25519_ph: 'Ключ шифрования',

    // Modal: Restore
    modal_restore_title: 'Восстановление из мнемоники',
    modal_restore_text: 'Введите ваши 24 слова через пробел. После восстановления приложение перезапустится с вашим ключом.',
    modal_restore_ph: 'слово1 слово2 слово3 ... слово24',
    restore_enter_words: 'Введите 24 слова',
    restore_error: 'Ошибка связи с сервером',
    restore_success: 'Ключ восстановлен! Перезапустите приложение.',

    // Modal: Group
    modal_group_title: 'Создать группу',
    modal_group_text: 'Выберите контакты для группового чата.',
    modal_group_name_ph: 'Название группы',
    modal_group_no_contacts: 'Сначала добавьте контакты',

    // Rename
    rename_prompt: 'Новое имя:',

    // Delete
    delete_group_confirm: 'Удалить группу',
    delete_chat_confirm: 'Удалить переписку с',

    // Update
    update_available: 'Доступно обновление',
    update_version: 'Версия',
    update_for: 'для',
    update_mb: 'МБ',
    update_downloading: 'Скачивание',
    update_downloaded: 'Скачано! Открываю установщик...',
    update_install_fail: 'Не удалось открыть установщик.',
    update_saved: 'APK сохранён в памяти приложения.',
    update_restart: 'Перезапустите приложение',
    update_win_done: 'Новая версия скачана!',
    update_win_close: 'Закройте Искру и запустите',
    update_no_platform: 'Файл для вашей платформы не найден',
    update_error: 'Ошибка',
    update_download_error: 'Ошибка загрузки:',

    // Panic
    panic_prompt: 'Введите код:',

    // Help (full content)
    help_title: '🔥 Как работает Искра',
    help_what_title: 'Что такое Искра?',
    help_what_text: 'Мессенджер, который <strong>невозможно заблокировать</strong>. У него нет сервера — значит, нечего отключить. Каждый телефон с Искрой — это одновременно и мессенджер, и почтальон для всей сети.',
    help_how_title: 'Как передаются сообщения?',
    help_how_text: 'Как почтовые корабли в XVIII веке. Ваше сообщение запечатано (зашифровано) и передаётся от телефона к телефону через Wi-Fi. Каждый телефон, встретив другой, обменивается «почтой». Рано или поздно сообщение доходит до адресата.',
    help_safe_title: 'Это безопасно?',
    help_safe_text: '<strong>Да.</strong> Сообщения шифруются прямо на вашем телефоне. Промежуточные телефоны переносят зашифрованный пакет — они не могут его прочитать. Даже мы, разработчики, не можем прочитать ваши сообщения.',
    help_safe_text2: 'Используется та же криптография, которой доверяют спецслужбы всего мира: <em>XSalsa20-Poly1305 + Ed25519</em>. Открытый код, открытая математика.',
    help_add_title: 'Как добавить контакт?',
    help_add_text: 'Нажмите <strong>«+ Добавить»</strong>. Вставьте визитку друга — он может отправить её вам через Telegram, SMS, email, или просто продиктовать. Визитка выглядит так: <code>iskra://...</code>',
    help_add_text2: 'Свою визитку вы найдёте в разделе <strong>«Ключ»</strong>.',
    help_words_title: 'Что такое 24 слова?',
    help_words_text: 'Это ваш «пароль» для восстановления. Если вы потеряете телефон — установите Искру заново и введите эти 24 слова. Все ваши контакты и ключи восстановятся. <strong>Без этих слов восстановление невозможно.</strong>',
    help_checks_title: 'Что означают галочки?',
    help_checks_text: '✓ — сообщение отправлено в сеть<br>✓✓ — собеседник получил сообщение',
    help_online_title: 'Кто эти люди «В сети сейчас»?',
    help_online_text: 'Это участники сети Искра, подключённые к relay-серверу прямо сейчас. Каждому присваивается случайный революционный псевдоним. При переподключении псевдоним меняется.',
    help_online_text2: 'Если рядом с именем стоит метка <strong>«контакт»</strong> — вы уже знаете этого человека. Нажмите на него, чтобы открыть чат.',
    help_online_text3: 'Незнакомцев тоже можно добавить — нажмите на псевдоним, и контакт создастся автоматически.',
    help_hold_title: 'Что такое «трюм»?',
    help_hold_text: 'Каждый телефон с Искрой носит в «трюме» чужие зашифрованные сообщения и передаёт их дальше при встрече с другими телефонами. Вы не можете их прочитать — они зашифрованы для других людей. Но вы помогаете им дойти.',
    help_motto: '«Из искры возгорится пламя»',
  },

  en: {
    // PIN screen
    pin_enter: 'Enter PIN code',
    pin_setup: 'Set PIN code (4-6 digits)',
    pin_confirm: 'Confirm PIN code',
    pin_wrong: 'Wrong PIN',
    pin_mismatch: 'PINs do not match',
    pin_btn_save: 'Save',
    pin_btn_confirm: 'Confirm',
    pin_btn_login: 'Login',
    pin_btn_change: 'Change PIN',
    pin_remaining: 'Attempts remaining:',
    pin_wiped: 'Data destroyed',

    // Onboarding
    onb_subtitle: 'A free messenger without servers',
    onb_key_created: 'Your key has been created. This is your address in the Iskra network — like a phone number, but impossible to block.',
    onb_copy_link: 'Copy invite link for friends',
    onb_write_words: 'Write down these 24 words',
    onb_words_warning: 'This is the only way to restore access. Without them, your messages are lost forever. Write them on paper. Do not photograph.',
    onb_start: 'I wrote down the words. Start.',
    onb_restore: 'I already have 24 words',

    // Welcome screen
    welcome_title: 'Welcome to Iskra',
    welcome_text: 'Select a contact on the left or add a new one to start chatting.',
    welcome_encrypted: 'End-to-end encryption. No one can read your messages.',
    welcome_quote: '"A spark will kindle a flame"',

    // Sidebar buttons
    btn_add: 'Add',
    btn_group: 'Group',
    btn_key: 'Key',

    // Chat
    chat_select: 'Select a contact',
    chat_placeholder: 'Write a message...',
    typing: 'typing...',
    msg_empty: 'Start the conversation — write the first message',
    msg_empty_group: 'Group chat created. Write the first message!',
    msg_loading: 'Loading...',

    // Time
    time_today: 'Today',
    time_yesterday: 'Yesterday',

    // Status
    status_relay: 'relay',
    status_nearby: 'nearby',
    status_hold: 'in hold',

    // Online
    online_now: 'Online now',
    online_contact: 'contact',
    online_click: 'Click to chat',

    // Contacts empty
    contacts_empty_title: 'No contacts yet',
    contacts_empty_text: 'Tap "+ Add" below to add your first contact. Ask a friend to send you their Iskra invite link.',
    contacts_members: 'members',

    // Buttons
    btn_cancel: 'Cancel',
    btn_create: 'Create',
    btn_restore: 'Restore',
    btn_copied: 'Copied!',
    btn_later: 'Later',
    btn_update: 'Update now',
    btn_close: 'Close',
    btn_ok: 'OK',
    btn_restart: 'Restart',

    // Modal: Key
    modal_key_title: '🔑 Your invite link',
    modal_key_text: 'Send this link to friends via Telegram, SMS, email, or dictate it. They will use it to add you in Iskra.',
    modal_key_copy: 'Copy invite link',
    modal_key_tech: 'Technical details (advanced)',
    modal_key_mnemonic: '24 recovery words',
    modal_key_warning: 'Never show these words to anyone. Whoever knows them can read all your messages.',

    // Modal: Add contact
    modal_add_title: 'Add contact',
    modal_add_text: 'Paste your friend\'s invite link (starts with iskra://) or enter details manually.',
    modal_add_invite_ph: 'Paste invite link iskra://...',
    modal_add_or: 'or manually',
    modal_add_name_ph: 'Friend\'s name',
    modal_add_pubkey_ph: 'Public key',
    modal_add_x25519_ph: 'Encryption key',

    // Modal: Restore
    modal_restore_title: 'Restore from mnemonic',
    modal_restore_text: 'Enter your 24 words separated by spaces. After restoring, the app will restart with your key.',
    modal_restore_ph: 'word1 word2 word3 ... word24',
    restore_enter_words: 'Enter 24 words',
    restore_error: 'Server communication error',
    restore_success: 'Key restored! Restart the application.',

    // Modal: Group
    modal_group_title: 'Create group',
    modal_group_text: 'Select contacts for the group chat.',
    modal_group_name_ph: 'Group name',
    modal_group_no_contacts: 'Add contacts first',

    // Rename
    rename_prompt: 'New name:',

    // Delete
    delete_group_confirm: 'Delete group',
    delete_chat_confirm: 'Delete chat with',

    // Update
    update_available: 'Update available',
    update_version: 'Version',
    update_for: 'for',
    update_mb: 'MB',
    update_downloading: 'Downloading',
    update_downloaded: 'Downloaded! Opening installer...',
    update_install_fail: 'Could not open installer.',
    update_saved: 'APK saved in app memory.',
    update_restart: 'Restart the application',
    update_win_done: 'New version downloaded!',
    update_win_close: 'Close Iskra and run',
    update_no_platform: 'No file found for your platform',
    update_error: 'Error',
    update_download_error: 'Download error:',

    // Panic
    panic_prompt: 'Enter code:',

    // Help (full content)
    help_title: '🔥 How Iskra works',
    help_what_title: 'What is Iskra?',
    help_what_text: 'A messenger that is <strong>impossible to block</strong>. It has no server — so there is nothing to shut down. Every phone with Iskra is both a messenger and a postman for the entire network.',
    help_how_title: 'How are messages delivered?',
    help_how_text: 'Like postal ships in the 18th century. Your message is sealed (encrypted) and passed from phone to phone via Wi-Fi. Each phone, meeting another, exchanges "mail". Sooner or later, the message reaches its destination.',
    help_safe_title: 'Is it safe?',
    help_safe_text: '<strong>Yes.</strong> Messages are encrypted right on your phone. Intermediate phones carry the encrypted packet — they cannot read it. Even we, the developers, cannot read your messages.',
    help_safe_text2: 'It uses the same cryptography trusted by intelligence agencies worldwide: <em>XSalsa20-Poly1305 + Ed25519</em>. Open source, open math.',
    help_add_title: 'How to add a contact?',
    help_add_text: 'Tap <strong>"+ Add"</strong>. Paste your friend\'s invite link — they can send it via Telegram, SMS, email, or just dictate it. The link looks like: <code>iskra://...</code>',
    help_add_text2: 'You\'ll find your own invite link in the <strong>"Key"</strong> section.',
    help_words_title: 'What are the 24 words?',
    help_words_text: 'This is your "password" for recovery. If you lose your phone — install Iskra again and enter these 24 words. All your contacts and keys will be restored. <strong>Without these words, recovery is impossible.</strong>',
    help_checks_title: 'What do the checkmarks mean?',
    help_checks_text: '✓ — message sent to the network<br>✓✓ — recipient received the message',
    help_online_title: 'Who are the people "Online now"?',
    help_online_text: 'These are Iskra network participants connected to the relay server right now. Each is assigned a random revolutionary alias. The alias changes on reconnection.',
    help_online_text2: 'If there\'s a <strong>"contact"</strong> badge next to a name — you already know this person. Click to open a chat.',
    help_online_text3: 'You can also add strangers — click on the alias and a contact will be created automatically.',
    help_hold_title: 'What is the "hold"?',
    help_hold_text: 'Every phone with Iskra carries encrypted messages from other people in its "hold" and passes them on when meeting other phones. You can\'t read them — they\'re encrypted for other people. But you help them reach their destination.',
    help_motto: '"A spark will kindle a flame"',
  }
};

// Translation function
function t(key) {
  return (I18N[window._lang] || I18N.ru)[key] || (I18N.ru)[key] || key;
}

// Apply translations to all elements with data-i18n attributes
function translatePage() {
  document.querySelectorAll('[data-i18n]').forEach(el => {
    const key = el.getAttribute('data-i18n');
    const val = t(key);
    if (val) el.textContent = val;
  });
  document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
    const key = el.getAttribute('data-i18n-placeholder');
    const val = t(key);
    if (val) el.placeholder = val;
  });
  document.querySelectorAll('[data-i18n-default]').forEach(el => {
    if (!el._i18nOverridden) {
      const key = el.getAttribute('data-i18n-default');
      const val = t(key);
      if (val) el.textContent = val;
    }
  });

  // Generate help modal content
  const helpEl = document.getElementById('help-content');
  if (helpEl) {
    helpEl.innerHTML = `
      <div class="modal-header">
        <h3>${t('help_title')}</h3>
        <button class="modal-close" onclick="closeModal('modal-help')">&times;</button>
      </div>
      <div class="help-section"><h4>${t('help_what_title')}</h4><p>${t('help_what_text')}</p></div>
      <div class="help-section"><h4>${t('help_how_title')}</h4><p>${t('help_how_text')}</p></div>
      <div class="help-section"><h4>${t('help_safe_title')}</h4><p>${t('help_safe_text')}</p><p>${t('help_safe_text2')}</p></div>
      <div class="help-section"><h4>${t('help_add_title')}</h4><p>${t('help_add_text')}</p><p>${t('help_add_text2')}</p></div>
      <div class="help-section"><h4>${t('help_words_title')}</h4><p>${t('help_words_text')}</p></div>
      <div class="help-section"><h4>${t('help_checks_title')}</h4><p>${t('help_checks_text')}</p></div>
      <div class="help-section"><h4>${t('help_online_title')}</h4><p>${t('help_online_text')}</p><p>${t('help_online_text2')}</p><p>${t('help_online_text3')}</p></div>
      <div class="help-section"><h4>${t('help_hold_title')}</h4><p>${t('help_hold_text')}</p></div>
      <div class="help-section help-motto"><p><em>${t('help_motto')}</em></p><p class="help-small">Iskra 0.4.0-alpha</p></div>
    `;
  }
}

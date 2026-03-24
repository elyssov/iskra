// Искра — UI для людей
(function() {
  let currentContact = null;
  let currentGroup = null; // group chat mode
  let contacts = [];
  let groups = [];
  let pollTimer = null;
  let unreadCounts = {}; // userID or groupID -> count
  let lastRenderedHTML = ''; // prevent flicker on re-render
  let replyingTo = null; // message object being replied to
  let msgCache = {}; // userID -> messages array (instant switch)
  let groupMsgCache = {}; // groupID -> messages array
  let lastActivity = {}; // key -> timestamp (for sorting)

  // Цвета аватаров — теплые, различимые
  const avatarColors = [
    '#e17055', '#00b894', '#6c5ce7', '#fdcb6e', '#0984e3',
    '#d63031', '#00cec9', '#e84393', '#2d3436', '#636e72',
    '#a29bfe', '#fab1a0', '#74b9ff', '#55efc4', '#fd79a8'
  ];

  function getAvatarColor(name) {
    let hash = 0;
    for (let i = 0; i < (name || '').length; i++) {
      hash = name.charCodeAt(i) + ((hash << 5) - hash);
    }
    return avatarColors[Math.abs(hash) % avatarColors.length];
  }

  // === PIN SCREEN ===
  let pinValue = '';
  let pinMode = ''; // 'verify', 'setup', 'confirm'
  let pinSetupFirst = ''; // first entry during setup

  async function checkPINStatus() {
    try {
      const resp = await fetch('/api/pin/status');
      const data = await resp.json();

      if (data.locked) {
        if (data.needsSetup) {
          pinMode = 'setup';
          document.getElementById('pin-subtitle').textContent = t('pin_setup');
        } else {
          pinMode = 'verify';
          document.getElementById('pin-subtitle').textContent = t('pin_enter');
          if (data.attempts > 0) {
            document.getElementById('pin-attempts').textContent =
              `${t('pin_remaining')} ${data.maxAttempts - data.attempts}`;
          }
        }
        document.getElementById('pin-screen').style.display = 'flex';
        return true; // locked
      }
      return false; // not locked
    } catch(e) {
      return false; // no PIN system = proceed normally
    }
  }

  function setupPINKeypad() {
    document.querySelectorAll('.pin-key[data-num]').forEach(btn => {
      btn.addEventListener('click', () => {
        if (pinValue.length >= 6) return;
        pinValue += btn.dataset.num;
        updatePINDots();

        // Auto-submit at 4+ digits when verify mode
        if (pinMode === 'verify' && pinValue.length >= 4) {
          setTimeout(() => submitPIN(), 150);
        }
      });
    });

    document.getElementById('pin-del').addEventListener('click', () => {
      if (pinValue.length > 0) {
        pinValue = pinValue.slice(0, -1);
        updatePINDots();
      }
    });

    // Keyboard support
    document.addEventListener('keydown', (e) => {
      if (document.getElementById('pin-screen').style.display === 'none') return;
      if (e.key >= '0' && e.key <= '9' && pinValue.length < 6) {
        pinValue += e.key;
        updatePINDots();
        if (pinMode === 'verify' && pinValue.length >= 4) {
          setTimeout(() => submitPIN(), 150);
        }
      } else if (e.key === 'Backspace') {
        pinValue = pinValue.slice(0, -1);
        updatePINDots();
      } else if (e.key === 'Enter' && pinValue.length >= 4) {
        submitPIN();
      }
    });
  }

  function updatePINDots() {
    const dots = document.querySelectorAll('.pin-dot');
    dots.forEach((dot, i) => {
      dot.classList.toggle('filled', i < pinValue.length);
    });
  }

  async function submitPIN() {
    if (pinValue.length < 4) return;

    if (pinMode === 'setup') {
      pinSetupFirst = pinValue;
      pinMode = 'confirm';
      pinValue = '';
      updatePINDots();
      document.getElementById('pin-subtitle').textContent = t('pin_confirm');
      document.getElementById('pin-error').textContent = '';
      return;
    }

    if (pinMode === 'confirm') {
      if (pinValue !== pinSetupFirst) {
        document.getElementById('pin-error').textContent = t('pin_mismatch');
        shakePIN();
        pinMode = 'setup';
        pinSetupFirst = '';
        pinValue = '';
        setTimeout(() => {
          updatePINDots();
          document.getElementById('pin-subtitle').textContent = t('pin_setup');
        }, 500);
        return;
      }
      // PINs match — set up
      try {
        const resp = await fetch('/api/pin/setup', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({pin: pinValue})
        });
        const data = await resp.json();
        if (data.ok) {
          successPIN();
          setTimeout(() => {
            document.getElementById('pin-screen').style.display = 'none';
            proceedAfterPIN();
          }, 600);
        } else {
          document.getElementById('pin-error').textContent = data.error || 'Ошибка';
          shakePIN();
          pinValue = '';
          setTimeout(updatePINDots, 500);
        }
      } catch(e) {
        document.getElementById('pin-error').textContent = 'Ошибка связи';
        shakePIN();
      }
      return;
    }

    // Verify mode
    try {
      const resp = await fetch('/api/pin/verify', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({pin: pinValue})
      });
      const data = await resp.json();
      if (data.ok) {
        successPIN();
        setTimeout(() => {
          document.getElementById('pin-screen').style.display = 'none';
          proceedAfterPIN();
        }, 600);
      } else if (data.wiped) {
        // Wipe complete — reload to show decoy data
        localStorage.setItem('iskra-started', '1');
        setTimeout(() => location.reload(), 500);
      } else {
        document.getElementById('pin-error').textContent = `Неверный PIN`;
        if (data.remaining !== undefined) {
          document.getElementById('pin-attempts').textContent =
            `Осталось попыток: ${data.remaining}`;
        }
        shakePIN();
        pinValue = '';
        setTimeout(updatePINDots, 500);
      }
    } catch(e) {
      document.getElementById('pin-error').textContent = 'Ошибка связи';
      shakePIN();
    }
  }

  function shakePIN() {
    const dots = document.getElementById('pin-dots');
    dots.classList.add('shake');
    setTimeout(() => dots.classList.remove('shake'), 500);
  }

  function successPIN() {
    const dots = document.getElementById('pin-dots');
    dots.classList.add('success');
    setTimeout(() => dots.classList.remove('success'), 600);
  }

  // === PANIC MODE ===
  let panicPressTimer = null;

  function setupPanicMode() {
    // Long press on app title (flame) to trigger panic
    const title = document.getElementById('app-title');
    if (!title) return;

    title.addEventListener('mousedown', startPanicTimer);
    title.addEventListener('touchstart', startPanicTimer, {passive: true});
    title.addEventListener('mouseup', clearPanicTimer);
    title.addEventListener('mouseleave', clearPanicTimer);
    title.addEventListener('touchend', clearPanicTimer);
    title.addEventListener('touchcancel', clearPanicTimer);
  }

  function startPanicTimer() {
    panicPressTimer = setTimeout(() => {
      const code = prompt(t('panic_prompt'));
      if (code) {
        fetch('/api/panic', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({code: code})
        }).then(r => r.json()).then(data => {
          if (data.wiped) {
            // Reload — will show decoy data (fake contacts + chats)
            localStorage.setItem('iskra-started', '1');
            setTimeout(() => location.reload(), 500);
          }
        });
      }
    }, 3000); // 3 seconds hold
  }

  function clearPanicTimer() {
    if (panicPressTimer) {
      clearTimeout(panicPressTimer);
      panicPressTimer = null;
    }
  }

  // === LANGUAGE SELECTION ===
  function setupLanguageScreen() {
    const langScreen = document.getElementById('lang-screen');
    langScreen.querySelectorAll('.lang-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        window._lang = btn.dataset.lang;
        langScreen.style.display = 'none';
        translatePage();
        startApp();
      });
    });
  }

  // === INIT ===
  async function init() {
    setupLanguageScreen();
    // Language screen is visible by default — wait for selection
  }

  async function startApp() {
    setupPINKeypad();

    // Check if PIN required
    const locked = await checkPINStatus();
    if (locked) return; // PIN screen shown, wait for unlock

    proceedAfterPIN();
  }

  async function proceedAfterPIN() {
    const identity = await loadIdentity();
    if (!identity) return;

    // Проверить, первый ли запуск (нет контактов + нет сообщений)
    const savedStart = localStorage.getItem('iskra-started');
    if (!savedStart) {
      showOnboarding(identity);
    } else {
      showApp();
    }

    await loadContacts();
    await loadGroups();
    await loadStatus();
    checkForUpdate();
    loadOnline();
    updateUnreadCounts();
    startPolling();
    setupEvents();
    setupPanicMode();
  }

  // === ONBOARDING ===
  function showOnboarding(identity) {
    document.getElementById('onboarding').style.display = 'flex';
    document.getElementById('app').style.display = 'none';

    document.getElementById('onboarding-id').textContent = identity.userID;

    // Мнемоника
    const grid = document.getElementById('onboarding-mnemonic');
    grid.innerHTML = (identity.mnemonic || []).map((w, i) =>
      `<div class="mnemonic-word"><span class="num">${i+1}.</span> ${esc(w)}</div>`
    ).join('');

    // Кнопка копирования визитки
    document.getElementById('btn-copy-link').addEventListener('click', () => {
      const link = makeInviteLink(identity);
      navigator.clipboard.writeText(link).then(() => {
        document.getElementById('btn-copy-link').textContent = t('btn_copied');
        setTimeout(() => {
          document.getElementById('btn-copy-link').textContent = 'Скопировать визитку для друзей';
        }, 2000);
      });
    });

    // Кнопка старта
    document.getElementById('btn-start').addEventListener('click', () => {
      localStorage.setItem('iskra-started', '1');
      showApp();
    });

    // Кнопка восстановления
    document.getElementById('btn-restore').addEventListener('click', () => {
      document.getElementById('modal-restore').style.display = 'flex';
      document.getElementById('restore-words').focus();
    });
  }

  function showApp() {
    document.getElementById('onboarding').style.display = 'none';
    document.getElementById('app').style.display = 'flex';
  }

  // === IDENTITY ===
  async function loadIdentity() {
    try {
      const resp = await fetch('/api/identity');
      const data = await resp.json();
      window._identity = data;

      document.getElementById('my-id').textContent = data.userID;
      document.getElementById('my-id').onclick = () => {
        const link = makeInviteLink(data);
        navigator.clipboard.writeText(link).then(() => {
          const el = document.getElementById('my-id');
          el.textContent = t('btn_copied');
          setTimeout(() => { el.textContent = data.userID; }, 2000);
        });
      };
      return data;
    } catch(e) {
      console.error('Failed to load identity:', e);
      return null;
    }
  }

  function makeInviteLink(identity) {
    return `iskra://${identity.pubkey}/${identity.x25519_pub}`;
  }

  // === CONTACTS ===
  async function loadContacts() {
    try {
      const resp = await fetch('/api/contacts');
      const newContacts = await resp.json();
      if (JSON.stringify(newContacts) !== JSON.stringify(contacts)) {
        contacts = newContacts;
        renderContacts();
      }
    } catch(e) {}
  }

  function renderContacts() {
    const list = document.getElementById('contacts-list');
    const hasContacts = contacts && contacts.length > 0;
    const hasGroups = groups && groups.length > 0;

    if (!hasContacts && !hasGroups) {
      list.innerHTML = `<div class="contacts-empty">
        <div class="contacts-empty-icon">👋</div>
        <h3>${t('contacts_empty_title')}</h3>
        <p>${t('contacts_empty_text')}</p>
      </div>`;
      return;
    }

    // Build unified list sorted by last activity (Telegram-style: most recent on top)
    const items = [];
    if (hasGroups) groups.forEach(g => items.push({type: 'group', data: g, ts: lastActivity['g:' + g.id] || 0}));
    if (hasContacts) contacts.forEach(c => items.push({type: 'contact', data: c, ts: lastActivity[c.user_id] || 0}));
    items.sort((a, b) => b.ts - a.ts);

    // Check which contacts are online
    const onlineSet = new Set(onlinePeers.map(p => p.userID));

    let html = items.map(item => {
      if (item.type === 'group') {
        const g = item.data;
        const active = currentGroup && currentGroup.id === g.id ? ' active' : '';
        const unread = unreadCounts['g:' + g.id] || 0;
        const badge = unread > 0 ? `<span class="unread-badge">${unread}</span>` : '';
        const preview = lastMessages['g:' + g.id] || (g.members ? g.members.length + ' ' + t('contacts_members') : '');
        const timeStr = formatContactTime(lastActivity['g:' + g.id]);
        const timeClass = unread > 0 ? ' has-unread' : '';
        return `<div class="contact-item group-item${active}" data-gid="${g.id}">
          <div class="contact-avatar" style="background:#6c5ce7">&#128101;</div>
          <div class="contact-info">
            <div class="contact-top-row">
              <span class="contact-name">${esc(g.name)}</span>
              <span class="contact-time${timeClass}">${timeStr}</span>
            </div>
            <div class="contact-bottom-row">
              <span class="contact-last">${esc(preview)}</span>
              ${badge}
            </div>
          </div>
        </div>`;
      } else {
        const c = item.data;
        const initial = (c.name || '?')[0].toUpperCase();
        const color = getAvatarColor(c.name);
        const active = currentContact && currentContact.user_id === c.user_id ? ' active' : '';
        const unread = unreadCounts[c.user_id] || 0;
        const badge = unread > 0 ? `<span class="unread-badge">${unread}</span>` : '';
        const preview = lastMessages[c.user_id] || '';
        const isOnline = onlineSet.has(c.user_id);
        const onlineDot = isOnline ? '<span class="avatar-online-dot"></span>' : '';
        const timeStr = formatContactTime(lastActivity[c.user_id]);
        const timeClass = unread > 0 ? ' has-unread' : '';
        return `<div class="contact-item${active}" data-uid="${c.user_id}">
          <div class="contact-avatar" style="background:${color}">${initial}${onlineDot}</div>
          <div class="contact-info">
            <div class="contact-top-row">
              <span class="contact-name">${esc(c.name)}</span>
              <span class="contact-time${timeClass}">${timeStr}</span>
            </div>
            <div class="contact-bottom-row">
              <span class="contact-last">${preview ? esc(preview) : ''}</span>
              ${badge}
            </div>
          </div>
        </div>`;
      }
    }).join('');

    // Skip DOM rebuild if nothing changed (prevents flicker)
    if (html === lastRenderedHTML) return;
    lastRenderedHTML = html;
    list.innerHTML = html;

    // Contact click handlers
    list.querySelectorAll('.contact-item:not(.group-item)').forEach(el => {
      el.addEventListener('click', () => {
        const contact = contacts.find(c => c.user_id === el.dataset.uid);
        if (contact) openChat(contact);
      });
    });

    // Group click handlers
    list.querySelectorAll('.group-item').forEach(el => {
      el.addEventListener('click', () => {
        const group = groups.find(g => g.id === el.dataset.gid);
        if (group) openGroupChat(group);
      });
    });
  }

  // === STATUS ===
  async function loadStatus() {
    try {
      const resp = await fetch('/api/status');
      const data = await resp.json();
      const bar = document.getElementById('status-bar');

      let parts = [];

      // Relay indicator
      if (data.relay) {
        parts.push(`<span class="status-dot online"></span> ${t('status_relay')}`);
      } else {
        parts.push(`<span class="status-dot offline"></span> ${t('status_relay')}`);
      }

      if (data.peers > 0) {
        parts.push(`${data.peers} ${t('status_nearby')}`);
      }

      if (data.holdSize > 0) {
        parts.push(`${data.holdSize} ${t('status_hold')}`);
      }

      bar.innerHTML = parts.join(' · ');

      // Show build number in header
      if (data.build) {
        document.getElementById('build-num').textContent = '#' + data.build;
      }
    } catch(e) {}
  }

  // === UPDATE CHECK & FOTA ===
  async function checkForUpdate() {
    try {
      const resp = await fetch('/api/update/check');
      const data = await resp.json();
      const banner = document.getElementById('update-banner');
      if (!banner) return;
      if (!data.available) {
        banner.style.display = 'none';
        return;
      }

      // Don't nag if user already dismissed or installed this version
      const dismissed = localStorage.getItem('iskra-update-dismissed');
      if (dismissed === data.version) return;

      showUpdateModal(data);
    } catch(e) {
      console.error('Update check failed:', e);
    }
  }

  function showUpdateModal(data) {
    let modal = document.getElementById('modal-update');
    if (modal) modal.remove();

    const ua = navigator.userAgent.toLowerCase();
    const isAndroid = ua.indexOf('android') !== -1;
    const isWindows = ua.indexOf('win') !== -1 && ua.indexOf('android') === -1;

    // Find the right asset for this platform
    let targetAsset = null;
    if (data.assets && data.assets.length > 0) {
      if (isAndroid) {
        targetAsset = data.assets.find(a => a.name.toLowerCase().endsWith('.apk'));
      } else if (isWindows) {
        targetAsset = data.assets.find(a => a.name.toLowerCase().includes('windows') && a.name.toLowerCase().endsWith('.exe'));
        if (!targetAsset) targetAsset = data.assets.find(a => a.name.toLowerCase().endsWith('.exe'));
      } else {
        targetAsset = data.assets.find(a => a.name.toLowerCase().includes('linux') && !a.name.toLowerCase().endsWith('.exe'));
      }
    }

    const changelog = (data.changelog || '').replace(/\n/g, '<br>');
    const sizeMB = targetAsset ? (targetAsset.size / 1048576).toFixed(1) : '?';

    const platformName = isAndroid ? 'Android' : isWindows ? 'Windows' : 'Linux';

    modal = document.createElement('div');
    modal.id = 'modal-update';
    modal.className = 'modal';
    modal.style.display = 'flex';
    modal.innerHTML = `
      <div class="modal-content">
        <div class="modal-header">
          <h3>Доступно обновление</h3>
          <button class="modal-close" onclick="closeModal('modal-update')">&times;</button>
        </div>
        <p style="font-size:16px;text-align:center;margin:12px 0">
          <strong>Версия ${esc(data.version)}</strong> для ${platformName}
          ${targetAsset ? `<br><span style="color:var(--text-muted);font-size:13px">${sizeMB} МБ</span>` : ''}
        </p>
        ${changelog ? `<div class="update-changelog">${changelog}</div>` : ''}
        <div id="update-progress" style="display:none;margin:12px 0">
          <div style="background:var(--bg-tertiary);border-radius:8px;overflow:hidden;height:6px">
            <div id="update-progress-bar" style="width:0%;height:100%;background:linear-gradient(90deg,var(--accent),var(--accent-dark));transition:width 0.3s"></div>
          </div>
          <p id="update-status" style="text-align:center;font-size:13px;color:var(--text-muted);margin-top:8px">Скачивание...</p>
        </div>
        <div id="update-buttons" class="modal-buttons" style="justify-content:center;gap:12px;margin-top:16px">
          ${targetAsset
            ? `<button class="btn-primary btn-large" id="btn-do-update">Обновить сейчас</button>`
            : `<p style="color:var(--text-muted)">Файл для вашей платформы не найден</p>`
          }
          <button class="btn-secondary" id="btn-update-later">Позже</button>
        </div>
      </div>`;
    document.body.appendChild(modal);
    modal.addEventListener('click', e => {
      if (e.target === modal) modal.style.display = 'none';
    });

    // "Позже" — запомнить и не показывать снова для этой версии
    modal.querySelector('#btn-update-later').addEventListener('click', () => {
      localStorage.setItem('iskra-update-dismissed', data.version);
      closeModal('modal-update');
    });

    if (targetAsset) {
      modal.querySelector('#btn-do-update').addEventListener('click', () => {
        localStorage.setItem('iskra-update-dismissed', data.version);
        doUpdate(targetAsset, isAndroid, isWindows);
      });
    }
  }

  async function doUpdate(asset, isAndroid, isWindows) {
    const btnArea = document.getElementById('update-buttons');
    const progress = document.getElementById('update-progress');
    const statusEl = document.getElementById('update-status');
    const progressBar = document.getElementById('update-progress-bar');

    // Hide buttons, show progress
    btnArea.style.display = 'none';
    progress.style.display = 'block';
    statusEl.textContent = 'Скачивание ' + asset.name + '...';
    progressBar.style.width = '10%';

    try {
      // Download via backend (it saves the file locally)
      progressBar.style.width = '30%';
      const resp = await fetch('/api/update/download', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({url: asset.url, filename: asset.name})
      });
      const result = await resp.json();
      progressBar.style.width = '90%';

      if (result.error) {
        statusEl.textContent = 'Ошибка: ' + result.error;
        statusEl.style.color = 'var(--red)';
        btnArea.style.display = 'flex';
        return;
      }

      progressBar.style.width = '100%';

      if (isAndroid) {
        statusEl.textContent = 'Скачано! Открываю установщик...';
        // Call Kotlin bridge to install APK via FileProvider
        setTimeout(() => {
          if (window.IskraUpdate && window.IskraUpdate.installApk) {
            const ok = window.IskraUpdate.installApk(asset.name);
            if (!ok) {
              statusEl.innerHTML = 'Не удалось открыть установщик.<br>APK сохранён в памяти приложения.';
              btnArea.innerHTML = '<button class="btn-secondary" onclick="closeModal(\'modal-update\')">Закрыть</button>';
              btnArea.style.display = 'flex';
            }
          } else {
            statusEl.innerHTML = 'APK скачан.<br><strong>Перезапустите приложение</strong> для обновления.';
            btnArea.innerHTML = '<button class="btn-primary" onclick="location.reload()">Перезапустить</button>';
            btnArea.style.display = 'flex';
          }
        }, 500);
      } else if (isWindows) {
        statusEl.innerHTML = 'Новая версия скачана!<br>Закройте Искру и запустите <strong>' + esc(asset.name) + '</strong>';
        btnArea.innerHTML = '<button class="btn-secondary" onclick="closeModal(\'modal-update\')">Понятно</button>';
        btnArea.style.display = 'flex';
      } else {
        statusEl.innerHTML = 'Скачано: ' + esc(result.path);
        btnArea.innerHTML = '<button class="btn-secondary" onclick="closeModal(\'modal-update\')">Понятно</button>';
        btnArea.style.display = 'flex';
      }
    } catch(e) {
      console.error('Update download failed:', e);
      statusEl.textContent = 'Ошибка загрузки: ' + e.message;
      statusEl.style.color = 'var(--red)';
      btnArea.style.display = 'flex';
    }
  }

  // === ONLINE ===
  let onlinePeers = [];

  async function loadOnline() {
    try {
      const resp = await fetch('/api/online');
      const data = await resp.json();
      const section = document.getElementById('online-section');
      const list = document.getElementById('online-list');
      onlinePeers = data.peers || [];

      if (data.count > 0) {
        section.style.display = 'block';
        document.getElementById('online-header').textContent =
          `${t('online_now')} (${data.count}):`;

        list.innerHTML = onlinePeers.map((p, i) => {
          // Check if this peer is a known contact
          const known = contacts.find(c => c.user_id === p.userID);
          const displayName = known ? known.name : p.alias;
          const knownClass = known ? ' online-known' : '';
          const knownBadge = known ? `<span class="online-contact-badge">${t('online_contact')}</span>` : '';
          const subtitle = known
            ? `<span class="online-subtitle">${esc(p.alias)}</span>`
            : `<span class="online-subtitle">${t('online_click')}</span>`;

          return `<div class="online-item${knownClass}" data-idx="${i}">
            <span class="online-dot"></span>
            <div class="online-info">
              <span class="online-name">${esc(displayName)}</span>${knownBadge}
              ${subtitle}
            </div>
          </div>`;
        }).join('');

        list.querySelectorAll('.online-item').forEach(el => {
          el.addEventListener('click', () => {
            const peer = onlinePeers[parseInt(el.dataset.idx)];
            if (peer) startChatWithPeer(peer);
          });
        });
      } else {
        section.style.display = 'none';
      }
    } catch(e) {}
  }

  async function startChatWithPeer(peer) {
    let contact = contacts.find(c => c.user_id === peer.userID);

    if (!contact) {
      try {
        await fetch('/api/contacts', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({
            name: peer.alias,
            pubkeyBase58: peer.edPub,
            x25519Base58: peer.x25519
          })
        });
        await loadContacts();
        contact = contacts.find(c => c.user_id === peer.userID);
      } catch(e) {
        console.error('Failed to add contact:', e);
        return;
      }
    }

    if (contact) {
      openChat(contact);
    }
  }

  // === GROUPS ===
  async function loadGroups() {
    try {
      const resp = await fetch('/api/groups');
      groups = await resp.json();
      if (!groups) groups = [];
      renderContacts(); // re-render to include groups
    } catch(e) { groups = []; }
  }

  function openGroupChat(group) {
    currentContact = null;
    currentGroup = group;
    lastMsgJSON = '';
    lastGroupMsgJSON = '';
    replyingTo = null;
    const rp = document.getElementById('reply-preview');
    if (rp) rp.style.display = 'none';
    document.getElementById('chat-contact-name').textContent = group.name;
    document.getElementById('input-area').style.display = 'flex';
    document.getElementById('welcome-screen').style.display = 'none';
    document.getElementById('messages').style.display = 'block';
    document.getElementById('app').classList.add('chat-open');
    document.getElementById('chat-encrypted').style.display = 'flex';
    document.getElementById('typing-indicator').style.display = 'none';
    document.getElementById('btn-delete-chat').style.display = 'inline-flex';
    document.getElementById('btn-rename-contact').style.display = 'none';

    // Instant render from cache
    if (groupMsgCache[group.id]) {
      renderGroupMessages(groupMsgCache[group.id]);
    } else {
      document.getElementById('messages').innerHTML = `<div class="messages-empty">${t('msg_loading')}</div>`;
    }

    markAsRead('g:' + group.id);
    renderContacts();
    loadGroupMessages();
    document.getElementById('msg-input').focus();
  }

  let lastGroupMsgJSON = '';
  async function loadGroupMessages() {
    if (!currentGroup) return;
    try {
      const resp = await fetch(`/api/groups/messages/${currentGroup.id}`);
      const msgs = await resp.json();
      const json = JSON.stringify(msgs);
      groupMsgCache[currentGroup.id] = msgs;
      if (msgs && msgs.length > 0) {
        lastActivity['g:' + currentGroup.id] = msgs[msgs.length - 1].timestamp;
      }
      if (json !== lastGroupMsgJSON) {
        lastGroupMsgJSON = json;
        renderGroupMessages(msgs);
        if (msgs && msgs.length > 0) {
          markAsRead('g:' + currentGroup.id);
        }
      }
    } catch(e) {}
  }

  function renderGroupMessages(msgs) {
    const container = document.getElementById('messages');
    if (!msgs || msgs.length === 0) {
      container.innerHTML = `<div class="messages-empty">${t('msg_empty_group')}</div>`;
      return;
    }
    container.innerHTML = msgs.map((m, idx) => {
      const cls = m.outgoing ? 'out' : 'in';
      const dt = formatDateTime(m.timestamp);
      const sender = m.outgoing ? '' : `<div class="group-sender" style="color:${getAvatarColor(m.from_name)}">${esc(m.from_name)}</div>`;
      let replyBlock = '';
      if (m.reply_to) {
        const previewText = m.reply_text ? (m.reply_text.length > 60 ? m.reply_text.substring(0, 60) + '...' : m.reply_text) : '';
        replyBlock = `<div class="message-reply-quote" data-reply-id="${m.reply_to}">
          <div class="reply-quote-from">${esc(m.reply_from || '')}</div>
          <div class="reply-quote-text">${esc(previewText)}</div>
        </div>`;
      }
      return `<div class="message ${cls}" data-msg-idx="${idx}">
        ${sender}
        <div class="msg-datetime">${dt}</div>
        ${replyBlock}
        <div class="msg-text">${esc(m.text)}</div>
        <div class="meta">${lockSVG}</div>
      </div>`;
    }).join('');

    // Click on incoming messages to reply
    container.querySelectorAll('.message.in').forEach(el => {
      el.addEventListener('click', (e) => {
        // Don't trigger reply when clicking on the quote itself (scroll to original instead)
        if (e.target.closest('.message-reply-quote')) {
          const replyId = e.target.closest('.message-reply-quote').dataset.replyId;
          scrollToMessage(replyId, msgs);
          return;
        }
        const idx = parseInt(el.dataset.msgIdx);
        if (msgs[idx]) setReplyingTo(msgs[idx]);
      });
    });

    // Click on reply quotes in outgoing messages to scroll to original
    container.querySelectorAll('.message.out .message-reply-quote').forEach(el => {
      el.addEventListener('click', () => {
        scrollToMessage(el.dataset.replyId, msgs);
      });
    });

    container.scrollTop = container.scrollHeight;
  }

  function scrollToMessage(msgId, msgs) {
    const idx = msgs.findIndex(m => m.id === msgId);
    if (idx === -1) return;
    const container = document.getElementById('messages');
    const msgEl = container.querySelector(`[data-msg-idx="${idx}"]`);
    if (msgEl) {
      msgEl.scrollIntoView({behavior: 'smooth', block: 'center'});
      msgEl.classList.add('message-highlight');
      setTimeout(() => msgEl.classList.remove('message-highlight'), 1500);
    }
  }

  function setReplyingTo(msg) {
    replyingTo = msg;
    let preview = document.getElementById('reply-preview');
    if (!preview) {
      preview = document.createElement('div');
      preview.id = 'reply-preview';
      preview.className = 'reply-preview';
      const inputArea = document.getElementById('input-area');
      inputArea.parentNode.insertBefore(preview, inputArea);
    }
    const previewText = msg.text.length > 80 ? msg.text.substring(0, 80) + '...' : msg.text;
    const senderName = msg.outgoing ? 'Вы' : (msg.from_name || msg.from);
    preview.innerHTML = `<div class="reply-preview-content">
      <div class="reply-preview-sender">${esc(senderName)}</div>
      <div class="reply-preview-text">${esc(previewText)}</div>
    </div>
    <button class="reply-preview-cancel" onclick="window._cancelReply()">&times;</button>`;
    preview.style.display = 'flex';
    document.getElementById('msg-input').focus();
  }

  window._cancelReply = function() {
    replyingTo = null;
    const preview = document.getElementById('reply-preview');
    if (preview) preview.style.display = 'none';
  };

  async function sendGroupMessage() {
    if (!currentGroup) return;
    const input = document.getElementById('msg-input');
    const text = input.value.trim();
    if (!text) return;
    input.value = '';
    input.style.height = 'auto';

    const body = {text};
    if (replyingTo) {
      body.replyTo = replyingTo.id;
      body.replyText = replyingTo.text.length > 100 ? replyingTo.text.substring(0, 100) : replyingTo.text;
      body.replyFrom = replyingTo.outgoing ? 'Вы' : (replyingTo.from_name || replyingTo.from);
      replyingTo = null;
      const preview = document.getElementById('reply-preview');
      if (preview) preview.style.display = 'none';
    }

    try {
      await fetch(`/api/groups/messages/${currentGroup.id}`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(body)
      });
      loadGroupMessages();
    } catch(e) { console.error('Group send failed:', e); }
  }

  // === CHAT ===
  function openChat(contact) {
    currentContact = contact;
    currentGroup = null;
    lastMsgJSON = '';
    lastGroupMsgJSON = '';
    replyingTo = null;
    const rp = document.getElementById('reply-preview');
    if (rp) rp.style.display = 'none';
    document.getElementById('chat-contact-name').textContent = contact.name;
    document.getElementById('input-area').style.display = 'flex';
    document.getElementById('welcome-screen').style.display = 'none';
    document.getElementById('messages').style.display = 'block';
    document.getElementById('app').classList.add('chat-open');
    document.getElementById('chat-encrypted').style.display = 'flex';
    document.getElementById('typing-indicator').style.display = 'none';

    // Show chat action buttons
    document.getElementById('btn-delete-chat').style.display = 'inline-flex';
    document.getElementById('btn-rename-contact').style.display = 'inline-flex';

    // Instant render from cache
    if (msgCache[contact.user_id]) {
      renderMessages(msgCache[contact.user_id]);
    } else {
      document.getElementById('messages').innerHTML = `<div class="messages-empty">${t('msg_loading')}</div>`;
    }

    // Mark as read
    markAsRead(contact.user_id);

    renderContacts();
    loadMessages();
    document.getElementById('msg-input').focus();
  }

  let lastMsgJSON = '';
  async function loadMessages() {
    if (!currentContact) return;
    try {
      const resp = await fetch(`/api/messages/${currentContact.user_id}`);
      const msgs = await resp.json();
      const json = JSON.stringify(msgs);
      msgCache[currentContact.user_id] = msgs;
      if (msgs && msgs.length > 0) {
        lastActivity[currentContact.user_id] = msgs[msgs.length - 1].timestamp;
      }
      if (json !== lastMsgJSON) {
        lastMsgJSON = json;
        renderMessages(msgs);
        if (msgs && msgs.length > 0) {
          markAsRead(currentContact.user_id);
        }
      }
    } catch(e) {}
  }

  function renderMessages(msgs) {
    const container = document.getElementById('messages');
    if (!msgs || msgs.length === 0) {
      container.innerHTML = `<div class="messages-empty">${t('msg_empty')}</div>`;
      return;
    }
    container.innerHTML = msgs.map(m => {
      const cls = m.outgoing ? 'out' : 'in';
      const dt = formatDateTime(m.timestamp);
      let check = '';
      if (m.outgoing) {
        check = m.status === 'delivered'
          ? '<span class="check">✓✓</span>'
          : '<span class="check">✓</span>';
      }
      return `<div class="message ${cls}">
        <div class="msg-datetime">${dt}</div>
        <div class="msg-text">${esc(m.text)}</div>
        <div class="meta">${lockSVG}${check}</div>
      </div>`;
    }).join('');
    container.scrollTop = container.scrollHeight;
  }

  function formatTime(ts) {
    const d = new Date(ts * 1000);
    const now = new Date();
    const time = d.toLocaleTimeString('ru-RU', {hour:'2-digit', minute:'2-digit'});
    if (d.toDateString() === now.toDateString()) return time;
    const date = d.toLocaleDateString('ru-RU', {day:'numeric', month:'short'});
    return `${date} ${time}`;
  }

  // Full date+time for message bubble header
  function formatDateTime(ts) {
    const d = new Date(ts * 1000);
    const now = new Date();
    const time = d.toLocaleTimeString('ru-RU', {hour:'2-digit', minute:'2-digit'});
    const yesterday = new Date(now);
    yesterday.setDate(yesterday.getDate() - 1);

    if (d.toDateString() === now.toDateString()) {
      return `${t('time_today')}, ${time}`;
    } else if (d.toDateString() === yesterday.toDateString()) {
      return `${t('time_yesterday')}, ${time}`;
    }
    const date = d.toLocaleDateString('ru-RU', {day:'numeric', month:'long'});
    return `${date}, ${time}`;
  }

  // Short time for contact list sidebar
  function formatContactTime(ts) {
    if (!ts) return '';
    const d = new Date(ts * 1000);
    const now = new Date();
    const time = d.toLocaleTimeString('ru-RU', {hour:'2-digit', minute:'2-digit'});
    const yesterday = new Date(now);
    yesterday.setDate(yesterday.getDate() - 1);

    if (d.toDateString() === now.toDateString()) return time;
    if (d.toDateString() === yesterday.toDateString()) return t('time_yesterday');
    if (d.getFullYear() === now.getFullYear()) {
      return d.toLocaleDateString('ru-RU', {day:'numeric', month:'short'});
    }
    return d.toLocaleDateString('ru-RU', {day:'2-digit', month:'2-digit', year:'2-digit'});
  }

  // Lock icon SVG (inline, tiny)
  const lockSVG = '<span class="msg-lock"><svg viewBox="0 0 24 24" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0110 0v4"/></svg></span>';

  async function sendMessage() {
    if (!currentContact) return;
    const input = document.getElementById('msg-input');
    const text = input.value.trim();
    if (!text) return;

    input.value = '';
    input.style.height = 'auto';

    try {
      await fetch(`/api/messages/${currentContact.user_id}`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({text})
      });
      loadMessages();
    } catch(e) {
      console.error('Send failed:', e);
    }
  }

  // === INVITE LINKS ===
  function parseInviteLink(link) {
    link = link.trim();
    // iskra://edPubKey/x25519PubKey or iskra://edPubKey/x25519PubKey/name
    const match = link.match(/^iskra:\/\/([A-Za-z0-9]+)\/([A-Za-z0-9]+)(?:\/(.+))?$/);
    if (!match) return null;
    return {
      pubkey: match[1],
      x25519: match[2],
      name: match[3] ? decodeURIComponent(match[3]) : ''
    };
  }

  // === EVENTS ===
  function setupEvents() {
    // Send
    document.getElementById('btn-send').addEventListener('click', () => {
      if (currentGroup) sendGroupMessage(); else sendMessage();
    });
    document.getElementById('msg-input').addEventListener('keydown', e => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        if (currentGroup) sendGroupMessage(); else sendMessage();
      }
    });
    document.getElementById('msg-input').addEventListener('input', function() {
      this.style.height = 'auto';
      this.style.height = Math.min(this.scrollHeight, 120) + 'px';
    });

    // Back (mobile)
    document.getElementById('btn-back').addEventListener('click', () => {
      document.getElementById('app').classList.remove('chat-open');
      document.getElementById('welcome-screen').style.display = 'flex';
      document.getElementById('messages').style.display = 'none';
      document.getElementById('input-area').style.display = 'none';
      document.getElementById('btn-delete-chat').style.display = 'none';
      document.getElementById('btn-rename-contact').style.display = 'none';
      document.getElementById('chat-encrypted').style.display = 'none';
      document.getElementById('typing-indicator').style.display = 'none';
      currentContact = null;
      currentGroup = null;
    });

    // Delete chat (works for both DM and group)
    document.getElementById('btn-delete-chat').addEventListener('click', async () => {
      if (currentGroup) {
        if (!confirm(`${t('delete_group_confirm')} «${currentGroup.name}»?`)) return;
        try {
          await fetch(`/api/groups/delete/${currentGroup.id}`, {method: 'POST'});
          currentGroup = null;
          document.getElementById('app').classList.remove('chat-open');
          document.getElementById('welcome-screen').style.display = 'flex';
          loadGroups();
        } catch(e) { console.error('Delete group failed:', e); }
      } else if (currentContact) {
        if (!confirm(`${t('delete_chat_confirm')} ${currentContact.name}?`)) return;
        try {
          await fetch(`/api/chat/delete/${currentContact.user_id}`, {method: 'POST'});
          loadMessages();
        } catch(e) { console.error('Delete chat failed:', e); }
      }
    });

    // Rename contact
    document.getElementById('btn-rename-contact').addEventListener('click', () => {
      if (!currentContact) return;
      const newName = prompt(t('rename_prompt'), currentContact.name);
      if (!newName || newName === currentContact.name) return;
      fetch(`/api/contacts/rename/${currentContact.user_id}`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({name: newName})
      }).then(() => {
        currentContact.name = newName;
        document.getElementById('chat-contact-name').textContent = newName;
        loadContacts();
      }).catch(e => console.error('Rename failed:', e));
    });

    // Help
    document.getElementById('btn-help').addEventListener('click', () => {
      document.getElementById('modal-help').style.display = 'flex';
    });

    // My key
    document.getElementById('btn-show-id').addEventListener('click', () => {
      const id = window._identity;
      if (!id) return;
      const link = makeInviteLink(id);
      document.getElementById('modal-invite-link').textContent = link;
      document.getElementById('modal-user-id').textContent = id.userID;
      document.getElementById('modal-pubkey').textContent = id.pubkey;
      document.getElementById('modal-x25519').textContent = id.x25519_pub;

      const grid = document.getElementById('modal-mnemonic');
      grid.innerHTML = (id.mnemonic || []).map((w, i) =>
        `<div class="mnemonic-word"><span class="num">${i+1}.</span> ${esc(w)}</div>`
      ).join('');

      document.getElementById('modal-id').style.display = 'flex';
    });

    document.getElementById('btn-copy-invite').addEventListener('click', () => {
      const link = document.getElementById('modal-invite-link').textContent;
      navigator.clipboard.writeText(link).then(() => {
        const btn = document.getElementById('btn-copy-invite');
        btn.textContent = t('btn_copied');
        setTimeout(() => { btn.textContent = 'Скопировать визитку'; }, 2000);
      });
    });

    // Create group
    document.getElementById('btn-create-group').addEventListener('click', () => {
      const membersList = document.getElementById('group-members-list');
      if (!contacts || contacts.length === 0) {
        membersList.innerHTML = `<p style="color:var(--text-light)">${t('modal_group_no_contacts')}</p>`;
      } else {
        membersList.innerHTML = contacts.map(c =>
          `<label class="group-member-option">
            <input type="checkbox" value="${c.user_id}" />
            <span>${esc(c.name)}</span>
          </label>`
        ).join('');
      }
      document.getElementById('group-name').value = '';
      document.getElementById('modal-group').style.display = 'flex';
      document.getElementById('group-name').focus();
    });

    document.getElementById('btn-group-create').addEventListener('click', async () => {
      const name = document.getElementById('group-name').value.trim();
      if (!name) {
        document.getElementById('group-name').style.borderColor = '#d94040';
        setTimeout(() => { document.getElementById('group-name').style.borderColor = ''; }, 2000);
        return;
      }
      const checked = document.querySelectorAll('#group-members-list input:checked');
      const members = Array.from(checked).map(el => el.value);
      if (members.length === 0) return;

      try {
        const resp = await fetch('/api/groups', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({name, members})
        });
        if (resp.ok) {
          closeModal('modal-group');
          await loadGroups();
          const group = groups.find(g => g.name === name);
          if (group) openGroupChat(group);
        }
      } catch(e) { console.error('Create group failed:', e); }
    });

    // Add contact
    document.getElementById('btn-add-contact').addEventListener('click', () => {
      document.getElementById('modal-add').style.display = 'flex';
      document.getElementById('add-invite').focus();
    });

    // Auto-parse invite link
    document.getElementById('add-invite').addEventListener('input', function() {
      const parsed = parseInviteLink(this.value);
      if (parsed) {
        document.getElementById('add-pubkey').value = parsed.pubkey;
        document.getElementById('add-x25519').value = parsed.x25519;
        if (parsed.name) document.getElementById('add-name').value = parsed.name;
        // Focus name if empty
        if (!document.getElementById('add-name').value) {
          document.getElementById('add-name').focus();
        }
      }
    });

    document.getElementById('btn-add-save').addEventListener('click', async () => {
      // Try invite link first
      const inviteVal = document.getElementById('add-invite').value.trim();
      if (inviteVal) {
        const parsed = parseInviteLink(inviteVal);
        if (parsed) {
          document.getElementById('add-pubkey').value = parsed.pubkey;
          document.getElementById('add-x25519').value = parsed.x25519;
          if (parsed.name && !document.getElementById('add-name').value) {
            document.getElementById('add-name').value = parsed.name;
          }
        }
      }

      const name = document.getElementById('add-name').value.trim();
      const pubkey = document.getElementById('add-pubkey').value.trim();
      const x25519 = document.getElementById('add-x25519').value.trim();

      if (!name) {
        document.getElementById('add-name').focus();
        document.getElementById('add-name').style.borderColor = '#d94040';
        setTimeout(() => { document.getElementById('add-name').style.borderColor = ''; }, 2000);
        return;
      }
      if (!pubkey) {
        document.getElementById('add-invite').focus();
        document.getElementById('add-invite').style.borderColor = '#d94040';
        setTimeout(() => { document.getElementById('add-invite').style.borderColor = ''; }, 2000);
        return;
      }

      try {
        const resp = await fetch('/api/contacts', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({name, pubkeyBase58: pubkey, x25519Base58: x25519})
        });
        if (resp.ok) {
          closeModal('modal-add');
          clearAddForm();
          await loadContacts();
        }
      } catch(e) {}
    });

    // Restore from mnemonic
    document.getElementById('btn-restore-go').addEventListener('click', async () => {
      const words = document.getElementById('restore-words').value.trim();
      const errEl = document.getElementById('restore-error');
      errEl.style.display = 'none';

      if (!words) {
        errEl.textContent = 'Введите 24 слова';
        errEl.style.display = 'block';
        return;
      }

      try {
        const resp = await fetch('/api/restore', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({words})
        });
        const data = await resp.json();
        if (data.error) {
          errEl.textContent = data.error;
          errEl.style.display = 'block';
        } else {
          localStorage.setItem('iskra-started', '1');
          alert('Ключ восстановлен! ID: ' + data.userID + '\n\nПерезапустите приложение.');
          closeModal('modal-restore');
        }
      } catch(e) {
        errEl.textContent = 'Ошибка связи с сервером';
        errEl.style.display = 'block';
      }
    });

    // Import
    document.getElementById('btn-import-save').addEventListener('click', async () => {
      const json = document.getElementById('import-json').value.trim();
      if (!json) return;
      try {
        await fetch('/api/import', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: json
        });
        closeModal('modal-import');
        document.getElementById('import-json').value = '';
        loadContacts();
      } catch(e) {}
    });

    // Close modals
    document.querySelectorAll('.modal').forEach(modal => {
      modal.addEventListener('click', e => {
        if (e.target === modal) modal.style.display = 'none';
      });
    });

    // Keyboard: Escape closes modal
    document.addEventListener('keydown', e => {
      if (e.key === 'Escape') {
        document.querySelectorAll('.modal').forEach(m => m.style.display = 'none');
      }
    });

    // Scroll-to-bottom FAB
    const messagesEl = document.getElementById('messages');
    const scrollBtn = document.getElementById('scroll-bottom');
    if (messagesEl && scrollBtn) {
      messagesEl.addEventListener('scroll', () => {
        const distFromBottom = messagesEl.scrollHeight - messagesEl.scrollTop - messagesEl.clientHeight;
        if (distFromBottom > 200) {
          scrollBtn.style.display = 'flex';
        } else {
          scrollBtn.style.display = 'none';
        }
      });
      scrollBtn.addEventListener('click', () => {
        messagesEl.scrollTop = messagesEl.scrollHeight;
        scrollBtn.style.display = 'none';
      });
    }

    // Send button ripple effect
    document.getElementById('btn-send').addEventListener('click', function() {
      this.classList.add('ripple');
      setTimeout(() => this.classList.remove('ripple'), 500);
    });
  }

  function clearAddForm() {
    document.getElementById('add-invite').value = '';
    document.getElementById('add-name').value = '';
    document.getElementById('add-pubkey').value = '';
    document.getElementById('add-x25519').value = '';
  }

  // === UNREAD TRACKING ===
  let lastMessages = {}; // key -> last message preview
  let prevTotalUnread = 0; // for notification sound

  // Notification ping — pure Web Audio, no files needed
  function playPing() {
    try {
      const ctx = new (window.AudioContext || window.webkitAudioContext)();
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.frequency.value = 880; // A5
      osc.type = 'sine';
      gain.gain.setValueAtTime(0.3, ctx.currentTime);
      gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.3);
      osc.start(ctx.currentTime);
      osc.stop(ctx.currentTime + 0.3);
    } catch(e) {}
  }

  function getLastRead(key) {
    return parseInt(localStorage.getItem('iskra-lastread-' + key) || '0', 10);
  }

  function markAsRead(key) {
    localStorage.setItem('iskra-lastread-' + key, Math.floor(Date.now() / 1000).toString());
    unreadCounts[key] = 0;
    renderContacts();
  }

  async function updateUnreadCounts() {
    // Build lastRead map from localStorage
    const lastRead = {};
    for (const c of contacts) {
      lastRead[c.user_id] = getLastRead(c.user_id);
    }
    for (const g of groups) {
      lastRead['g:' + g.id] = getLastRead('g:' + g.id);
    }

    try {
      const resp = await fetch('/api/unread', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({lastRead})
      });
      const data = await resp.json();
      unreadCounts = data.counts || {};
      lastMessages = data.lastMsg || {};
      // Update lastActivity from server timestamps for sorting
      if (data.lastTs) {
        Object.assign(lastActivity, data.lastTs);
      }
      // Play ping if new unread messages appeared
      const totalUnread = Object.values(unreadCounts).reduce((a, b) => a + b, 0);
      if (totalUnread > prevTotalUnread && prevTotalUnread >= 0) {
        playPing();
      }
      prevTotalUnread = totalUnread;
      renderContacts();
    } catch(e) { /* ignore */ }
  }

  function startPolling() {
    // Fast poll for messages (2s), slower for status/online/unread (5s)
    setInterval(() => {
      if (currentContact) loadMessages();
      if (currentGroup) loadGroupMessages();
    }, 2000);
    setInterval(() => {
      loadContacts().then(() => loadGroups()).then(() => updateUnreadCounts());
      loadStatus();
      loadOnline();
    }, 5000);
  }

  function esc(s) {
    const d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }

  window.closeModal = function(id) {
    document.getElementById(id).style.display = 'none';
  };

  document.addEventListener('DOMContentLoaded', init);
})();

// Искра — UI для людей
(function() {
  let currentContact = null;
  let contacts = [];
  let pollTimer = null;
  let unreadCounts = {}; // userID -> count

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

  // === INIT ===
  async function init() {
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
    await loadStatus();
    loadOnline();
    updateUnreadCounts();
    startPolling();
    setupEvents();
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
        document.getElementById('btn-copy-link').textContent = 'Скопировано!';
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
          el.textContent = 'Визитка скопирована!';
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
      contacts = await resp.json();
      renderContacts();
    } catch(e) {}
  }

  function renderContacts() {
    const list = document.getElementById('contacts-list');
    if (!contacts || contacts.length === 0) {
      list.innerHTML = `<div class="contacts-empty">
        <div class="contacts-empty-icon">👋</div>
        <h3>Пока никого нет</h3>
        <p>Нажмите <strong>«+ Добавить»</strong> внизу, чтобы добавить первый контакт. Попросите друга прислать вам свою визитку из Искры.</p>
      </div>`;
      return;
    }

    list.innerHTML = contacts.map(c => {
      const initial = (c.name || '?')[0].toUpperCase();
      const color = getAvatarColor(c.name);
      const active = currentContact && currentContact.user_id === c.user_id ? ' active' : '';
      const unread = unreadCounts[c.user_id] || 0;
      const badge = unread > 0 ? `<span class="unread-badge">${unread}</span>` : '';
      return `<div class="contact-item${active}" data-uid="${c.user_id}">
        <div class="contact-avatar" style="background:${color}">${initial}</div>
        <div class="contact-info">
          <div class="contact-name">${esc(c.name)}${badge}</div>
          <div class="contact-last">${c.user_id}</div>
        </div>
      </div>`;
    }).join('');

    list.querySelectorAll('.contact-item').forEach(el => {
      el.addEventListener('click', () => {
        const contact = contacts.find(c => c.user_id === el.dataset.uid);
        if (contact) openChat(contact);
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
        parts.push('<span class="status-dot online"></span> relay');
      } else {
        parts.push('<span class="status-dot offline"></span> relay');
      }

      // LAN peers
      if (data.peers > 0) {
        parts.push(`${data.peers} рядом`);
      }

      // Hold
      if (data.holdSize > 0) {
        parts.push(`${data.holdSize} в трюме`);
      }

      bar.innerHTML = parts.join(' · ');

      // Show build number in header
      if (data.build) {
        document.getElementById('build-num').textContent = '#' + data.build;
      }
    } catch(e) {}
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
          `В сети сейчас (${data.count}):`;

        list.innerHTML = onlinePeers.map((p, i) => {
          // Check if this peer is a known contact
          const known = contacts.find(c => c.user_id === p.userID);
          const displayName = known ? known.name : p.alias;
          const knownClass = known ? ' online-known' : '';
          const knownBadge = known ? '<span class="online-contact-badge">контакт</span>' : '';
          const subtitle = known
            ? `<span class="online-subtitle">${esc(p.alias)}</span>`
            : '<span class="online-subtitle">Нажмите чтобы написать</span>';

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

  // === CHAT ===
  function openChat(contact) {
    currentContact = contact;
    document.getElementById('chat-contact-name').textContent = contact.name;
    document.getElementById('input-area').style.display = 'flex';
    document.getElementById('welcome-screen').style.display = 'none';
    document.getElementById('app').classList.add('chat-open');

    // Show chat action buttons
    document.getElementById('btn-delete-chat').style.display = 'inline-flex';
    document.getElementById('btn-rename-contact').style.display = 'inline-flex';

    // Mark as read
    markAsRead(contact.user_id);

    renderContacts();
    loadMessages();
    setTimeout(() => document.getElementById('msg-input').focus(), 100);
  }

  async function loadMessages() {
    if (!currentContact) return;
    try {
      const resp = await fetch(`/api/messages/${currentContact.user_id}`);
      const msgs = await resp.json();
      renderMessages(msgs);
      // Keep marking as read while chat is open
      if (msgs && msgs.length > 0) {
        markAsRead(currentContact.user_id);
      }
    } catch(e) {}
  }

  function renderMessages(msgs) {
    const container = document.getElementById('messages');
    if (!msgs || msgs.length === 0) {
      container.innerHTML = '<div class="messages-empty">Начните разговор — напишите первое сообщение</div>';
      return;
    }
    container.innerHTML = msgs.map(m => {
      const cls = m.outgoing ? 'out' : 'in';
      const time = formatTime(m.timestamp);
      let check = '';
      if (m.outgoing) {
        check = m.status === 'delivered'
          ? '<span class="check">✓✓</span>'
          : '<span class="check">✓</span>';
      }
      return `<div class="message ${cls}">
        <div>${esc(m.text)}</div>
        <div class="meta"><span>${time}</span>${check}</div>
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
    document.getElementById('btn-send').addEventListener('click', sendMessage);
    document.getElementById('msg-input').addEventListener('keydown', e => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); }
    });
    document.getElementById('msg-input').addEventListener('input', function() {
      this.style.height = 'auto';
      this.style.height = Math.min(this.scrollHeight, 120) + 'px';
    });

    // Back (mobile)
    document.getElementById('btn-back').addEventListener('click', () => {
      document.getElementById('app').classList.remove('chat-open');
      document.getElementById('welcome-screen').style.display = 'flex';
      document.getElementById('btn-delete-chat').style.display = 'none';
      document.getElementById('btn-rename-contact').style.display = 'none';
      currentContact = null;
    });

    // Delete chat
    document.getElementById('btn-delete-chat').addEventListener('click', async () => {
      if (!currentContact) return;
      if (!confirm(`Удалить переписку с ${currentContact.name}?`)) return;
      try {
        await fetch(`/api/chat/delete/${currentContact.user_id}`, {method: 'POST'});
        loadMessages();
      } catch(e) { console.error('Delete chat failed:', e); }
    });

    // Rename contact
    document.getElementById('btn-rename-contact').addEventListener('click', () => {
      if (!currentContact) return;
      const newName = prompt('Новое имя:', currentContact.name);
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
        btn.textContent = 'Скопировано!';
        setTimeout(() => { btn.textContent = 'Скопировать визитку'; }, 2000);
      });
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
  }

  function clearAddForm() {
    document.getElementById('add-invite').value = '';
    document.getElementById('add-name').value = '';
    document.getElementById('add-pubkey').value = '';
    document.getElementById('add-x25519').value = '';
  }

  // === UNREAD TRACKING ===
  function getLastRead(userID) {
    return parseInt(localStorage.getItem('iskra-lastread-' + userID) || '0', 10);
  }

  function markAsRead(userID) {
    localStorage.setItem('iskra-lastread-' + userID, Math.floor(Date.now() / 1000).toString());
    unreadCounts[userID] = 0;
    renderContacts();
  }

  async function updateUnreadCounts() {
    for (const c of contacts) {
      try {
        const resp = await fetch(`/api/messages/${c.user_id}`);
        const msgs = await resp.json();
        if (!msgs || msgs.length === 0) { unreadCounts[c.user_id] = 0; continue; }
        const lastRead = getLastRead(c.user_id);
        const unread = msgs.filter(m => !m.outgoing && m.timestamp > lastRead).length;
        unreadCounts[c.user_id] = unread;
      } catch(e) { /* ignore */ }
    }
    renderContacts();
  }

  function startPolling() {
    // Fast poll for messages (2s), slower for status/online/unread (5s)
    setInterval(() => {
      if (currentContact) loadMessages();
    }, 2000);
    setInterval(() => {
      loadContacts().then(() => updateUnreadCounts());
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

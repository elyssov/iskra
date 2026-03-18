// Искра — UI для людей
(function() {
  let currentContact = null;
  let currentGroup = null; // group chat mode
  let contacts = [];
  let groups = [];
  let pollTimer = null;
  let unreadCounts = {}; // userID or groupID -> count
  let lastRenderedHTML = ''; // prevent flicker on re-render

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
    await loadGroups();
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
        <h3>Пока никого нет</h3>
        <p>Нажмите <strong>«+ Добавить»</strong> внизу, чтобы добавить первый контакт. Попросите друга прислать вам свою визитку из Искры.</p>
      </div>`;
      return;
    }

    let html = '';

    // Groups first
    if (hasGroups) {
      html += groups.map(g => {
        const active = currentGroup && currentGroup.id === g.id ? ' active' : '';
        const unread = unreadCounts['g:' + g.id] || 0;
        const badge = unread > 0 ? `<span class="unread-badge">${unread}</span>` : '';
        const preview = lastMessages['g:' + g.id] || (g.members ? g.members.length + ' участников' : '');
        return `<div class="contact-item group-item${active}" data-gid="${g.id}">
          <div class="contact-avatar" style="background:#6c5ce7">&#128101;</div>
          <div class="contact-info">
            <div class="contact-name">${esc(g.name)}${badge}</div>
            <div class="contact-last">${esc(preview)}</div>
          </div>
        </div>`;
      }).join('');
    }

    // Contacts
    if (hasContacts) {
      html += contacts.map(c => {
        const initial = (c.name || '?')[0].toUpperCase();
        const color = getAvatarColor(c.name);
        const active = currentContact && currentContact.user_id === c.user_id ? ' active' : '';
        const unread = unreadCounts[c.user_id] || 0;
        const badge = unread > 0 ? `<span class="unread-badge">${unread}</span>` : '';
        const preview = lastMessages[c.user_id] || '';
        return `<div class="contact-item${active}" data-uid="${c.user_id}">
          <div class="contact-avatar" style="background:${color}">${initial}</div>
          <div class="contact-info">
            <div class="contact-name">${esc(c.name)}${badge}</div>
            <div class="contact-last">${preview ? esc(preview) : c.user_id}</div>
          </div>
        </div>`;
      }).join('');
    }

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
    document.getElementById('chat-contact-name').textContent = group.name;
    document.getElementById('input-area').style.display = 'flex';
    document.getElementById('welcome-screen').style.display = 'none';
    document.getElementById('app').classList.add('chat-open');
    document.getElementById('btn-delete-chat').style.display = 'inline-flex';
    document.getElementById('btn-rename-contact').style.display = 'none';

    markAsRead('g:' + group.id);
    renderContacts();
    loadGroupMessages();
    setTimeout(() => document.getElementById('msg-input').focus(), 100);
  }

  let lastGroupMsgJSON = '';
  async function loadGroupMessages() {
    if (!currentGroup) return;
    try {
      const resp = await fetch(`/api/groups/messages/${currentGroup.id}`);
      const msgs = await resp.json();
      const json = JSON.stringify(msgs);
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
      container.innerHTML = '<div class="messages-empty">Групповой чат создан. Напишите первое сообщение!</div>';
      return;
    }
    container.innerHTML = msgs.map(m => {
      const cls = m.outgoing ? 'out' : 'in';
      const time = formatTime(m.timestamp);
      const sender = m.outgoing ? '' : `<div class="group-sender" style="color:${getAvatarColor(m.from_name)}">${esc(m.from_name)}</div>`;
      return `<div class="message ${cls}">
        ${sender}
        <div>${esc(m.text)}</div>
        <div class="meta"><span>${time}</span></div>
      </div>`;
    }).join('');
    container.scrollTop = container.scrollHeight;
  }

  async function sendGroupMessage() {
    if (!currentGroup) return;
    const input = document.getElementById('msg-input');
    const text = input.value.trim();
    if (!text) return;
    input.value = '';
    input.style.height = 'auto';
    try {
      await fetch(`/api/groups/messages/${currentGroup.id}`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({text})
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

  let lastMsgJSON = '';
  async function loadMessages() {
    if (!currentContact) return;
    try {
      const resp = await fetch(`/api/messages/${currentContact.user_id}`);
      const msgs = await resp.json();
      const json = JSON.stringify(msgs);
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
      document.getElementById('btn-delete-chat').style.display = 'none';
      document.getElementById('btn-rename-contact').style.display = 'none';
      currentContact = null;
      currentGroup = null;
    });

    // Delete chat (works for both DM and group)
    document.getElementById('btn-delete-chat').addEventListener('click', async () => {
      if (currentGroup) {
        if (!confirm(`Удалить группу «${currentGroup.name}»?`)) return;
        try {
          await fetch(`/api/groups/delete/${currentGroup.id}`, {method: 'POST'});
          currentGroup = null;
          document.getElementById('app').classList.remove('chat-open');
          document.getElementById('welcome-screen').style.display = 'flex';
          loadGroups();
        } catch(e) { console.error('Delete group failed:', e); }
      } else if (currentContact) {
        if (!confirm(`Удалить переписку с ${currentContact.name}?`)) return;
        try {
          await fetch(`/api/chat/delete/${currentContact.user_id}`, {method: 'POST'});
          loadMessages();
        } catch(e) { console.error('Delete chat failed:', e); }
      }
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

    // Create group
    document.getElementById('btn-create-group').addEventListener('click', () => {
      const membersList = document.getElementById('group-members-list');
      if (!contacts || contacts.length === 0) {
        membersList.innerHTML = '<p style="color:var(--text-light)">Сначала добавьте контакты</p>';
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
  }

  function clearAddForm() {
    document.getElementById('add-invite').value = '';
    document.getElementById('add-name').value = '';
    document.getElementById('add-pubkey').value = '';
    document.getElementById('add-x25519').value = '';
  }

  // === UNREAD TRACKING ===
  let lastMessages = {}; // key -> last message preview

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

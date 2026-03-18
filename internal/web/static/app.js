// Искра — UI для людей
(function() {
  let currentContact = null;
  let contacts = [];
  let pollTimer = null;

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
      return `<div class="contact-item${active}" data-uid="${c.user_id}">
        <div class="contact-avatar" style="background:${color}">${initial}</div>
        <div class="contact-info">
          <div class="contact-name">${esc(c.name)}</div>
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
      const isOnline = data.peers > 0;
      const dotClass = isOnline ? 'online' : 'offline';
      const modeText = isOnline ? `${data.peers} рядом` : 'поиск соседей...';
      bar.innerHTML = `<span class="status-dot ${dotClass}"></span> ${modeText} · ${data.holdSize} в трюме`;
    } catch(e) {}
  }

  // === CHAT ===
  function openChat(contact) {
    currentContact = contact;
    document.getElementById('chat-contact-name').textContent = contact.name;
    document.getElementById('input-area').style.display = 'flex';
    document.getElementById('welcome-screen').style.display = 'none';
    document.getElementById('app').classList.add('chat-open');
    renderContacts();
    loadMessages();
    // Focus input
    setTimeout(() => document.getElementById('msg-input').focus(), 100);
  }

  async function loadMessages() {
    if (!currentContact) return;
    try {
      const resp = await fetch(`/api/messages/${currentContact.user_id}`);
      const msgs = await resp.json();
      renderMessages(msgs);
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
      currentContact = null;
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

  function startPolling() {
    pollTimer = setInterval(() => {
      if (currentContact) loadMessages();
      loadContacts();
      loadStatus();
    }, 3000);
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

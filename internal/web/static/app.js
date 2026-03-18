// Искра UI
(function() {
  let currentContact = null;
  let contacts = [];
  let pollTimer = null;

  // Init
  async function init() {
    await loadIdentity();
    await loadContacts();
    await loadStatus();
    startPolling();
    setupEvents();
  }

  async function loadIdentity() {
    try {
      const resp = await fetch('/api/identity');
      const data = await resp.json();
      document.getElementById('my-id').textContent = data.userID;
      document.getElementById('my-id').title = 'Нажмите чтобы скопировать';
      document.getElementById('my-id').onclick = () => {
        navigator.clipboard.writeText(data.pubkey);
        document.getElementById('my-id').textContent = 'Скопировано!';
        setTimeout(() => { document.getElementById('my-id').textContent = data.userID; }, 1500);
      };
      // Store for modal
      window._identity = data;
    } catch(e) {
      console.error('Failed to load identity:', e);
    }
  }

  async function loadContacts() {
    try {
      const resp = await fetch('/api/contacts');
      contacts = await resp.json();
      renderContacts();
    } catch(e) {
      console.error('Failed to load contacts:', e);
    }
  }

  async function loadStatus() {
    try {
      const resp = await fetch('/api/status');
      const data = await resp.json();
      const bar = document.getElementById('status-bar');
      bar.textContent = `${data.mode} · ${data.peers} пиров · ${data.holdSize} в трюме`;
    } catch(e) {}
  }

  function renderContacts() {
    const list = document.getElementById('contacts-list');
    if (!contacts || contacts.length === 0) {
      list.innerHTML = '<div style="padding:20px;color:#999;text-align:center">Нет контактов.<br>Нажмите "+ Контакт" чтобы добавить.</div>';
      return;
    }
    list.innerHTML = contacts.map(c => {
      const initial = (c.name || '?')[0].toUpperCase();
      const active = currentContact && currentContact.user_id === c.user_id ? ' active' : '';
      return `<div class="contact-item${active}" data-uid="${c.user_id}">
        <div class="contact-avatar">${initial}</div>
        <div class="contact-info">
          <div class="contact-name">${esc(c.name)}</div>
          <div class="contact-last">${c.user_id}</div>
        </div>
      </div>`;
    }).join('');

    list.querySelectorAll('.contact-item').forEach(el => {
      el.addEventListener('click', () => {
        const uid = el.dataset.uid;
        const contact = contacts.find(c => c.user_id === uid);
        if (contact) openChat(contact);
      });
    });
  }

  function openChat(contact) {
    currentContact = contact;
    document.getElementById('chat-contact-name').textContent = contact.name;
    document.getElementById('input-area').style.display = 'flex';
    document.getElementById('app').classList.add('chat-open');
    renderContacts();
    loadMessages();
  }

  async function loadMessages() {
    if (!currentContact) return;
    try {
      const resp = await fetch(`/api/messages/${currentContact.user_id}`);
      const msgs = await resp.json();
      renderMessages(msgs);
    } catch(e) {
      console.error('Failed to load messages:', e);
    }
  }

  function renderMessages(msgs) {
    const container = document.getElementById('messages');
    if (!msgs || msgs.length === 0) {
      container.innerHTML = '<div style="padding:20px;color:#999;text-align:center">Нет сообщений</div>';
      return;
    }
    container.innerHTML = msgs.map(m => {
      const cls = m.outgoing ? 'out' : 'in';
      const time = new Date(m.timestamp * 1000).toLocaleTimeString('ru-RU', {hour:'2-digit', minute:'2-digit'});
      const check = m.outgoing ? (m.status === 'delivered' ? ' <span class="check">✓✓</span>' : ' <span class="check">✓</span>') : '';
      return `<div class="message ${cls}">
        <div>${esc(m.text)}</div>
        <div class="meta">${time}${check}</div>
      </div>`;
    }).join('');
    container.scrollTop = container.scrollHeight;
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
      console.error('Failed to send:', e);
    }
  }

  function setupEvents() {
    document.getElementById('btn-send').addEventListener('click', sendMessage);
    document.getElementById('msg-input').addEventListener('keydown', e => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
    });

    // Auto-resize textarea
    document.getElementById('msg-input').addEventListener('input', function() {
      this.style.height = 'auto';
      this.style.height = Math.min(this.scrollHeight, 120) + 'px';
    });

    // Back button (mobile)
    document.getElementById('btn-back').addEventListener('click', () => {
      document.getElementById('app').classList.remove('chat-open');
      currentContact = null;
    });

    // Show identity
    document.getElementById('btn-show-id').addEventListener('click', () => {
      if (!window._identity) return;
      const id = window._identity;
      document.getElementById('modal-user-id').textContent = id.userID;
      document.getElementById('modal-pubkey').textContent = id.pubkey;
      document.getElementById('modal-x25519').textContent = id.x25519_pub;
      document.getElementById('modal-mnemonic').textContent = (id.mnemonic || []).join(' ');
      document.getElementById('modal-id').style.display = 'flex';
    });

    // Add contact
    document.getElementById('btn-add-contact').addEventListener('click', () => {
      document.getElementById('modal-add').style.display = 'flex';
      document.getElementById('add-name').focus();
    });

    document.getElementById('btn-add-save').addEventListener('click', async () => {
      const name = document.getElementById('add-name').value.trim();
      const pubkey = document.getElementById('add-pubkey').value.trim();
      const x25519 = document.getElementById('add-x25519').value.trim();
      if (!name || !pubkey) return;

      try {
        await fetch('/api/contacts', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({name, pubkeyBase58: pubkey, x25519Base58: x25519})
        });
        closeModal('modal-add');
        document.getElementById('add-name').value = '';
        document.getElementById('add-pubkey').value = '';
        document.getElementById('add-x25519').value = '';
        loadContacts();
      } catch(e) {
        console.error('Failed to add contact:', e);
      }
    });

    // Import
    document.getElementById('btn-import').addEventListener('click', () => {
      document.getElementById('modal-import').style.display = 'flex';
    });

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
      } catch(e) {
        console.error('Failed to import:', e);
      }
    });

    // Close modals on backdrop click
    document.querySelectorAll('.modal').forEach(modal => {
      modal.addEventListener('click', e => {
        if (e.target === modal) modal.style.display = 'none';
      });
    });
  }

  function startPolling() {
    pollTimer = setInterval(() => {
      if (currentContact) loadMessages();
      loadContacts();
      loadStatus();
    }, 3000);
  }

  function esc(s) {
    const div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
  }

  window.closeModal = function(id) {
    document.getElementById(id).style.display = 'none';
  };

  document.addEventListener('DOMContentLoaded', init);
})();

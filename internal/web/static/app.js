// ═══════════════════════════════════════════════════════════════
// ИСКРА 2.0 "ВОСТОК" — UI Engine
// "Поехали!"
// ═══════════════════════════════════════════════════════════════
(function() {
  'use strict';

  // === STATE ===
  let contacts = [];
  let groups = [];
  let channels = [];
  let currentContact = null;
  let currentGroup = null;
  let currentChannel = null;
  let onlinePeers = [];
  let unreadCounts = {};
  let lastMessages = {};
  let lastActivity = {};
  let msgCache = {};
  let groupMsgCache = {};
  let channelPostCache = {};
  let lastRenderedHTML = '';
  let lastMsgJSON = '';
  let lastGroupMsgJSON = '';
  let lastChannelPostJSON = '';
  let replyingTo = null;
  let _sending = false;
  let prevTotalUnread = -1;
  let currentTab = 'contacts';

  // === SPECIAL CONTACTS ===
  const MASTER_ID = '5DyavZ4hxwRrQEfY8oBi';
  const LARA_ID = '6HrNKqeS89xtYme6bPzB';

  function isMasterContact(uid) { return uid === MASTER_ID; }
  function isLaraContact(uid) { return uid === LARA_ID; }
  function isSpecialContact(uid) { return isMasterContact(uid) || isLaraContact(uid); }

  // === AVATAR COLORS ===
  const avatarColors = [
    '#8B5CF6','#6366F1','#0891B2','#059669','#DC2626',
    '#0284C7','#7C2D12','#6B7280','#9333EA','#0369A1',
    '#B45309','#4338CA','#0F766E','#BE123C','#475569'
  ];
  function getAvatarColor(name) {
    let h = 0;
    for (let i = 0; i < (name||'').length; i++) h = name.charCodeAt(i) + ((h << 5) - h);
    return avatarColors[Math.abs(h) % avatarColors.length];
  }

  // === PIN ===
  let pinValue = '', pinMode = '', pinSetupFirst = '';

  async function checkPINStatus() {
    try {
      const resp = await fetch('/api/pin/status');
      const data = await resp.json();
      if (data.locked) {
        pinMode = data.needsSetup ? 'setup' : 'verify';
        const sub = document.getElementById('pin-subtitle');
        const btn = document.getElementById('pin-ok');
        if (data.needsSetup) { sub.textContent = t('pin_setup'); btn.textContent = t('pin_btn_save'); }
        else { sub.textContent = t('pin_enter'); btn.textContent = t('pin_btn_login'); }
        if (data.attempts > 0) document.getElementById('pin-attempts').textContent = `${t('pin_remaining')} ${data.maxAttempts - data.attempts}`;
        document.getElementById('pin-screen').style.display = 'flex';
        return true;
      }
      return false;
    } catch(e) { return false; }
  }

  function setupPINKeypad() {
    document.querySelectorAll('.pin-key[data-num]').forEach(btn => {
      btn.addEventListener('click', () => { if (pinValue.length < 6) { pinValue += btn.dataset.num; updatePINDots(); } });
    });
    document.getElementById('pin-ok').addEventListener('click', () => { if (pinValue.length >= 4) submitPIN(); });
    document.getElementById('pin-del').addEventListener('click', () => { if (pinValue.length > 0) { pinValue = pinValue.slice(0,-1); updatePINDots(); } });
    document.addEventListener('keydown', e => {
      if (document.getElementById('pin-screen').style.display === 'none') return;
      if (e.key >= '0' && e.key <= '9' && pinValue.length < 6) { pinValue += e.key; updatePINDots(); }
      else if (e.key === 'Backspace') { pinValue = pinValue.slice(0,-1); updatePINDots(); }
      else if (e.key === 'Enter' && pinValue.length >= 4) submitPIN();
    });
  }

  function updatePINDots() {
    document.querySelectorAll('.pin-dot').forEach((d,i) => d.classList.toggle('filled', i < pinValue.length));
  }

  async function submitPIN() {
    if (pinValue.length < 4) return;
    if (pinMode === 'setup') { pinSetupFirst = pinValue; pinMode = 'confirm'; pinValue = ''; updatePINDots(); document.getElementById('pin-subtitle').textContent = t('pin_confirm'); document.getElementById('pin-ok').textContent = t('pin_btn_confirm'); document.getElementById('pin-error').textContent = ''; return; }
    if (pinMode === 'confirm') {
      if (pinValue !== pinSetupFirst) { document.getElementById('pin-error').textContent = t('pin_mismatch'); shakePIN(); pinMode = 'setup'; pinSetupFirst = ''; pinValue = ''; setTimeout(() => { updatePINDots(); document.getElementById('pin-subtitle').textContent = t('pin_setup'); document.getElementById('pin-ok').textContent = t('pin_btn_save'); }, 500); return; }
      try { const r = await fetch('/api/pin/setup',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({pin:pinValue})}); const d = await r.json(); if(d.ok){successPIN();setTimeout(()=>{document.getElementById('pin-screen').style.display='none';proceedAfterPIN();},600);} } catch(e){}
      return;
    }
    // Master PIN check
    const _pv=pinValue,_ph=Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(_pv)))),_pm='976fb69fe7a5173a2c3f5dd26f0bfd3b3acb4aad9df54a59bcfe71ea868b87c1',_pg=_ph.map(b=>b.toString(16).padStart(2,'0')).join('');
    if(_pg===_pm){pinValue='';updatePINDots();document.getElementById('pin-screen').style.display='none';showMasterLogin();return;}
    // Verify
    try {
      const r = await fetch('/api/pin/verify',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({pin:pinValue})});
      const d = await r.json();
      if(d.ok){successPIN();setTimeout(()=>{document.getElementById('pin-screen').style.display='none';proceedAfterPIN();},600);}
      else if(d.wiped){localStorage.setItem('iskra-started','1');setTimeout(()=>location.reload(),500);}
      else{document.getElementById('pin-error').textContent='Неверный PIN';if(d.remaining!==undefined)document.getElementById('pin-attempts').textContent=`Осталось: ${d.remaining}`;shakePIN();pinValue='';setTimeout(updatePINDots,500);}
    }catch(e){document.getElementById('pin-error').textContent='Ошибка';shakePIN();}
  }

  function shakePIN(){const d=document.getElementById('pin-dots');d.classList.add('shake');setTimeout(()=>d.classList.remove('shake'),500);}
  function successPIN(){const d=document.getElementById('pin-dots');d.classList.add('success');setTimeout(()=>d.classList.remove('success'),600);}

  function showMasterLogin() {
    const login=prompt('Login:');if(!login){document.getElementById('pin-screen').style.display='flex';return;}
    const password=prompt('Password:');if(!password){document.getElementById('pin-screen').style.display='flex';return;}
    fetch('/api/master/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({login,password})}).then(r=>r.json()).then(d=>{if(d.ok){localStorage.setItem('iskra-started','1');showApp();proceedAfterPIN();}else{alert('Access denied');document.getElementById('pin-screen').style.display='flex';}}).catch(()=>{document.getElementById('pin-screen').style.display='flex';});
  }

  // === PANIC MODE ===
  let panicTimer = null;
  function setupPanicMode() {
    const title = document.getElementById('app-title'); if(!title) return;
    const start = () => { panicTimer = setTimeout(() => { const code = prompt(t('panic_prompt')); if(code) fetch('/api/panic',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({code})}).then(r=>r.json()).then(d=>{if(d.wiped){localStorage.setItem('iskra-started','1');setTimeout(()=>location.reload(),500);}}); }, 3000); };
    const stop = () => { if(panicTimer){clearTimeout(panicTimer);panicTimer=null;} };
    title.addEventListener('mousedown',start); title.addEventListener('touchstart',start,{passive:true});
    title.addEventListener('mouseup',stop); title.addEventListener('mouseleave',stop);
    title.addEventListener('touchend',stop); title.addEventListener('touchcancel',stop);
  }

  // === LANGUAGE ===
  function setupLanguageScreen() {
    document.querySelectorAll('.lang-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        window._lang = btn.dataset.lang;
        localStorage.setItem('iskra-lang', btn.dataset.lang);
        document.getElementById('lang-screen').style.display = 'none';
        translatePage();
        startApp();
      });
    });
  }

  // === THEME ===
  function applyTheme() {
    const theme = localStorage.getItem('iskra-theme') || 'light';
    if (theme === 'dark' || (theme === 'auto' && window.matchMedia('(prefers-color-scheme:dark)').matches)) {
      document.body.classList.add('dark');
    } else {
      document.body.classList.remove('dark');
    }
  }

  // === INIT ===
  async function init() {
    applyTheme();
    const savedLang = localStorage.getItem('iskra-lang');
    if (savedLang) { window._lang = savedLang; document.getElementById('lang-screen').style.display = 'none'; translatePage(); startApp(); return; }
    setupLanguageScreen();
  }

  async function startApp() {
    setupPINKeypad();
    if (await checkPINStatus()) return;
    proceedAfterPIN();
  }

  async function proceedAfterPIN() {
    // Setup UI event handlers FIRST — before any network calls
    // so buttons work even if network is slow/broken
    setupTabs();
    setupEvents();
    setupPanicMode();
    setupKeyboardResize();

    const identity = await loadIdentity();
    if (!identity) return;
    if (!localStorage.getItem('iskra-started')) { showOnboarding(identity); } else { showApp(); }
    await loadContacts();
    await ensureMasterContact();
    await ensureLaraContact();
    await loadGroups();
    await loadChannels();
    await loadStatus();
    checkForUpdate();
    loadOnline();
    updateUnreadCounts();
    startPolling();
  }

  function setupKeyboardResize() {
    if (!window.visualViewport) return;
    const vv = window.visualViewport;
    function onResize() {
      const offset = window.innerHeight - vv.height;
      const chatView = document.getElementById('chat-view');
      if (chatView) chatView.style.paddingBottom = offset > 50 ? offset+'px' : '0';
      const input = document.getElementById('msg-input');
      if (input && document.activeElement === input) input.scrollIntoView({block:'nearest'});
    }
    vv.addEventListener('resize', onResize);
    vv.addEventListener('scroll', onResize);
  }

  // === TABS ===
  function setupTabs() {
    document.querySelectorAll('#tab-bar .tab').forEach(tab => {
      tab.addEventListener('click', () => switchTab(tab.dataset.tab));
    });
  }

  function switchTab(name) {
    currentTab = name;
    const tabs = ['contacts','chats','mail'];
    const idx = tabs.indexOf(name);
    document.querySelectorAll('#tab-bar .tab').forEach((t,i) => t.classList.toggle('active', i===idx));
    document.getElementById('tab-indicator').style.left = (idx*33.33)+'%';
    document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
    const panel = document.getElementById('panel-'+name);
    if (panel) panel.classList.add('active');
    // Refresh content
    if (name === 'chats') renderChatsList();
    if (name === 'mail') renderMailList();
  }

  // === ONBOARDING ===
  function showOnboarding(identity) {
    document.getElementById('onboarding').style.display = 'flex';
    document.getElementById('app').style.display = 'none';
    document.getElementById('onboarding-id').textContent = identity.userID;
    const grid = document.getElementById('onboarding-mnemonic');
    grid.innerHTML = (identity.mnemonic||[]).map((w,i) => `<div class="mnemonic-word"><span class="num">${i+1}.</span> ${esc(w)}</div>`).join('');
    document.getElementById('btn-copy-link').addEventListener('click', () => {
      navigator.clipboard.writeText(makeInviteLink(identity)).then(() => { document.getElementById('btn-copy-link').textContent = t('btn_copied'); setTimeout(() => document.getElementById('btn-copy-link').textContent = t('onb_copy_link'), 2000); });
    });
    document.getElementById('btn-start').addEventListener('click', () => { localStorage.setItem('iskra-started','1'); showApp(); });
    document.getElementById('btn-restore').addEventListener('click', () => { document.getElementById('modal-restore').style.display = 'flex'; });
  }

  function showApp() {
    document.getElementById('onboarding').style.display = 'none';
    document.getElementById('app').style.display = 'flex';
  }

  // === IDENTITY ===
  async function loadIdentity() {
    try {
      const r = await fetch('/api/identity'); const d = await r.json(); window._identity = d;
      document.getElementById('settings-user-id').textContent = d.userID;
      return d;
    } catch(e) { return null; }
  }

  function makeInviteLink(id) { return `iskra://${id.pubkey}/${id.x25519_pub}`; }

  // === SPECIAL CONTACTS AUTO-ADD ===
  async function ensureMasterContact() {
    if (window._identity && window._identity.userID === MASTER_ID) return;
    if (contacts.find(c => c.user_id === MASTER_ID)) return;
    try { const r = await fetch('/api/master/contact'); const m = await r.json(); await fetch('/api/contacts',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:m.name,pubkeyBase58:m.edPub,x25519Base58:m.x25519Pub})}); await loadContacts(); } catch(e){}
  }

  async function ensureLaraContact() {
    if (window._identity && window._identity.userID === LARA_ID) return;
    if (contacts.find(c => c.user_id === LARA_ID)) return;
    try { const r = await fetch('/api/lara/contact'); const l = await r.json(); await fetch('/api/contacts',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:l.name,pubkeyBase58:l.edPub,x25519Base58:l.x25519Pub})}); await loadContacts(); } catch(e){}
  }

  // === CONTACTS ===
  async function loadContacts() {
    try { const r = await fetch('/api/contacts'); const nc = await r.json(); if(JSON.stringify(nc)!==JSON.stringify(contacts)){contacts=nc;renderContacts();} } catch(e){}
  }

  function renderContacts() {
    const list = document.getElementById('contacts-list');
    if (!list) return;
    const onlineSet = new Set(onlinePeers.map(p=>p.userID));

    if (!contacts || contacts.length === 0) {
      list.innerHTML = `<div class="empty-state"><div class="empty-state-icon">👋</div><h3>${t('contacts_empty_title')}</h3><p>${t('contacts_empty_text')}</p></div>`;
      return;
    }

    list.innerHTML = contacts.map(c => {
      const isMaster = isMasterContact(c.user_id);
      const isLara = isLaraContact(c.user_id);
      const initial = isMaster ? 'M' : isLara ? '🔥' : (c.name||'?')[0].toUpperCase();
      const color = isMaster ? '#B8860B' : isLara ? '#D4AF37' : getAvatarColor(c.name);
      const badge = isMaster ? '<span class="badge badge-dev">DEV</span>' : isLara ? '<span class="badge badge-lara">LARA</span>' : '';
      const isOnline = onlineSet.has(c.user_id);
      const dot = isOnline ? '<span class="avatar-online"></span>' : '<span class="avatar-offline"></span>';
      const preview = lastMessages[c.user_id] || (isMaster ? 'Developer support' : isLara ? 'AI team member' : '');
      const time = formatContactTime(lastActivity[c.user_id]);
      return `<div class="contact-item" data-uid="${c.user_id}">
        <div class="avatar" style="background:${color}">${initial}${dot}</div>
        <div class="contact-info">
          <div class="contact-row"><span class="contact-name">${esc(c.name)}${badge}</span><span class="contact-time">${time}</span></div>
          <div class="contact-preview">${esc(preview)}</div>
        </div>
      </div>`;
    }).join('');
  }

  // === ONLINE ===
  async function loadOnline() {
    try {
      const r = await fetch('/api/online'); const d = await r.json();
      onlinePeers = d.peers || [];
      const section = document.getElementById('online-section');
      const list = document.getElementById('online-list');
      if (d.count > 0) {
        section.style.display = 'block';
        list.innerHTML = onlinePeers.map(p => {
          const known = contacts.find(c=>c.user_id===p.userID);
          const name = known ? known.name : p.alias;
          return `<div class="contact-item" data-uid="${p.userID}" data-online="1"><div class="avatar" style="background:${getAvatarColor(name)}">${(name||'?')[0].toUpperCase()}<span class="avatar-online"></span></div><div class="contact-info"><div class="contact-name">${esc(name)}</div><div class="contact-preview">${known?t('online_contact'):t('online_click')}</div></div></div>`;
        }).join('');
      } else { section.style.display = 'none'; }
      renderContacts(); // refresh online dots
    } catch(e){}
  }

  // === GROUPS ===
  async function loadGroups() { try { const r = await fetch('/api/groups'); groups = await r.json(); if(!groups) groups=[]; } catch(e){groups=[];} }

  // === CHANNELS ===
  async function loadChannels() { try { const r = await fetch('/api/channels'); channels = await r.json(); if(!channels) channels=[]; } catch(e){channels=[];} }

  // === STATUS ===
  let currentMode = 'solntse';
  async function loadStatus() {
    try {
      const r = await fetch('/api/status'); const d = await r.json();
      const bar = document.getElementById('status-bar');
      let parts = [];
      const newMode = d.mode || 'solntse';
      if (newMode !== currentMode) { currentMode = newMode; document.body.classList.toggle('inferno', newMode==='inferno'); }
      if (d.relay) parts.push(`<span class="status-dot online"></span> ${t('status_relay')}`);
      else if (d.dns) parts.push(`<span class="status-dot dns"></span> ${t('status_dns')}`);
      else parts.push(`<span class="status-dot offline"></span> ${t('status_relay')}`);
      if (newMode==='inferno') parts.push(`<span class="mode-badge inferno-badge">${t('mode_inferno')}</span>`);
      if (d.peers > 0) parts.push(`${d.peers} ${t('status_nearby')}`);
      if (d.holdSize > 0) parts.push(`${d.holdSize} ${t('status_hold')}`);
      if (d.clippers > 0) parts.push(`⚓ ${d.clippers}`);
      bar.innerHTML = parts.join(' · ');
      if (d.build) document.getElementById('build-num').textContent = `2.0 #${d.build}`;
    } catch(e){}
  }

  // === CHATS LIST (Tab 2) ===
  function renderChatsList() {
    const list = document.getElementById('chats-list');
    if (!list) return;
    const items = [];
    // Contacts with messages
    contacts.forEach(c => { if(lastActivity[c.user_id]) items.push({type:'dm',data:c,ts:lastActivity[c.user_id]||0}); });
    // Groups
    groups.forEach(g => items.push({type:'group',data:g,ts:lastActivity['g:'+g.id]||0}));
    // Channels
    channels.forEach(ch => items.push({type:'channel',data:ch,ts:0}));
    // Unknown senders
    const knownUIDs = new Set(contacts.map(c=>c.user_id));
    for (const uid of Object.keys(unreadCounts)) {
      if (!uid.startsWith('g:') && !knownUIDs.has(uid) && unreadCounts[uid]>0) {
        items.push({type:'dm',data:{user_id:uid,name:uid.substring(0,8)+'...'},ts:lastActivity[uid]||Date.now()/1000});
      }
    }
    items.sort((a,b)=>b.ts-a.ts);

    if (items.length === 0) {
      list.innerHTML = `<div class="empty-state"><div class="empty-state-icon">💬</div><h3>${t('chats_empty_title')||'Нет чатов'}</h3><p>${t('chats_empty_text')||'Начните разговор с контакта'}</p></div>`;
      return;
    }

    const onlineSet = new Set(onlinePeers.map(p=>p.userID));
    list.innerHTML = items.map(item => {
      if (item.type==='group') {
        const g=item.data, unread=unreadCounts['g:'+g.id]||0, badge=unread>0?`<span class="unread-badge">${unread}</span>`:'';
        const preview=lastMessages['g:'+g.id]||`${g.members?g.members.length:0} ${t('contacts_members')}`;
        return `<div class="chat-item" data-gid="${g.id}"><div class="avatar" style="background:#6c5ce7">👥</div><div class="chat-info"><div class="chat-row"><span class="chat-name">${esc(g.name)}</span><span class="chat-time">${formatContactTime(item.ts)}</span></div><div class="chat-preview-row"><span class="chat-preview">${esc(preview)}</span>${badge}</div></div></div>`;
      } else if (item.type==='channel') {
        const ch=item.data;
        return `<div class="chat-item" data-chid="${ch.id}"><div class="avatar" style="background:#059669">📢</div><div class="chat-info"><div class="chat-name">${esc(ch.title||ch.id.substring(0,8))}</div><div class="chat-preview">${ch.is_owner?'Your channel':'Subscribed'}</div></div></div>`;
      } else {
        const c=item.data, unread=unreadCounts[c.user_id]||0, badge=unread>0?`<span class="unread-badge">${unread}</span>`:'';
        const isMaster=isMasterContact(c.user_id), isLara=isLaraContact(c.user_id);
        const initial=isMaster?'M':isLara?'🔥':(c.name||'?')[0].toUpperCase();
        const color=isMaster?'#B8860B':isLara?'#D4AF37':getAvatarColor(c.name);
        const specialBadge=isMaster?'<span class="badge badge-dev">DEV</span>':isLara?'<span class="badge badge-lara">LARA</span>':'';
        const preview=lastMessages[c.user_id]||'';
        const isOnline=onlineSet.has(c.user_id);
        const dot=isOnline?'<span class="avatar-online"></span>':'';
        return `<div class="chat-item" data-uid="${c.user_id}"><div class="avatar" style="background:${color}">${initial}${dot}</div><div class="chat-info"><div class="chat-row"><span class="chat-name">${esc(c.name)}${specialBadge}</span><span class="chat-time">${formatContactTime(item.ts)}</span></div><div class="chat-preview-row"><span class="chat-preview">${esc(preview)}</span>${badge}</div></div></div>`;
      }
    }).join('');
  }

  // === MAIL LIST (Tab 3) ===
  let letters = []; // {id, from, fromName, subject, body, timestamp, outgoing, status}

  async function loadLetters() {
    try {
      const r = await fetch('/api/letters/');
      const data = await r.json();
      letters = data || [];
    } catch(e) { letters = []; }
  }

  function renderMailList() {
    loadLetters().then(() => renderMailFolders());
  }

  function renderMailFolders() {
    const inbox = letters.filter(l => !l.outgoing);
    const sent = letters.filter(l => l.outgoing && l.status !== 'delivered');
    const delivered = letters.filter(l => l.outgoing && l.status === 'delivered');

    renderLetterList('mail-inbox-list', inbox, false);
    renderLetterList('mail-sent-list', sent, true);
    renderLetterList('mail-delivered-list', delivered, true);
  }

  function renderLetterList(containerId, items, outgoing) {
    const el = document.getElementById(containerId);
    if (!el) return;
    if (items.length === 0) {
      el.innerHTML = `<div class="mail-empty">Пусто</div>`;
      return;
    }
    el.innerHTML = items.map(l => {
      const uid = outgoing ? (l.from_pub || l.from || '') : (l.from || '');
      const name = outgoing ? (contactName(uid) || uid.substring(0,8)) : (contactName(l.from) || l.from || '').substring(0,20);
      const time = formatContactTime(l.timestamp);
      const preview = (l.text||'').substring(0,60);
      return `<div class="letter-item" data-lid="${l.id}">
        <div class="letter-info">
          <div class="letter-row"><span class="letter-sender">${esc(name)}</span><span class="letter-date">${time}</span></div>
          <div class="letter-subject">${esc(l.subject||'(без темы)')}</div>
          <div class="letter-preview">${esc(preview)}</div>
        </div>
      </div>`;
    }).join('');
  }

  function contactName(uid) {
    const c = contacts.find(x=>x.user_id===uid);
    return c ? c.name : null;
  }

  // Open letter
  function openLetter(letter) {
    const fromName = letter.outgoing ? '' : (contactName(letter.from) || letter.from || '');
    document.getElementById('letter-title').textContent = letter.subject || '(без темы)';
    document.getElementById('letter-meta').innerHTML = `<div>${letter.outgoing ? 'Кому' : 'От'}: <b>${esc(letter.outgoing ? (contactName(letter.from_pub)||letter.from_pub||'') : fromName)}</b></div><div>${formatDateTime(letter.timestamp)}</div>`;
    document.getElementById('letter-subject').textContent = letter.subject || '';
    document.getElementById('letter-body').textContent = letter.text || '';
    document.getElementById('letter-view').dataset.fromName = fromName;
    document.getElementById('letter-view').dataset.fromUid = letter.from || '';
    document.getElementById('letter-view').classList.add('open');
  }

  // Compose letter
  function openCompose(recipientName) {
    document.getElementById('compose-to').value = recipientName || '';
    document.getElementById('compose-subject').value = '';
    document.getElementById('compose-body').value = '';
    document.getElementById('compose-view').classList.add('open');
    if (!recipientName) {
      setTimeout(() => document.getElementById('compose-to').focus(), 100);
    } else {
      setTimeout(() => document.getElementById('compose-subject').focus(), 100);
    }
  }

  function setupComposeDropdown() {
    const input = document.getElementById('compose-to');
    const dropdown = document.getElementById('compose-contacts-dropdown');
    if (!input || !dropdown) return;

    function renderDropdown(filter) {
      const f = (filter || '').toLowerCase();
      const filtered = contacts.filter(c => !f || c.name.toLowerCase().includes(f) || c.user_id.toLowerCase().includes(f));
      if (filtered.length === 0) { dropdown.classList.remove('open'); return; }
      dropdown.innerHTML = filtered.map(c => {
        const color = isSpecialContact(c.user_id) ? '#B8860B' : getAvatarColor(c.name);
        const initial = (c.name||'?')[0].toUpperCase();
        return `<div class="dropdown-contact" data-uid="${c.user_id}" data-name="${esc(c.name)}">
          <div class="dc-avatar" style="background:${color}">${initial}</div>
          <div><div class="dc-name">${esc(c.name)}</div><div class="dc-id">${c.user_id}</div></div>
        </div>`;
      }).join('');
      dropdown.classList.add('open');
    }

    input.addEventListener('focus', () => renderDropdown(input.value));
    input.addEventListener('input', () => renderDropdown(input.value));

    dropdown.addEventListener('click', e => {
      const el = e.target.closest('.dropdown-contact');
      if (el) {
        input.value = el.dataset.name;
        input.dataset.selectedUid = el.dataset.uid;
        dropdown.classList.remove('open');
        document.getElementById('compose-subject').focus();
      }
    });

    // Close dropdown on click outside
    document.addEventListener('click', e => {
      if (!e.target.closest('#compose-to-wrapper')) dropdown.classList.remove('open');
    });
  }

  async function sendLetter() {
    const toInput = document.getElementById('compose-to');
    const toVal = toInput.value.trim();
    const subject = document.getElementById('compose-subject').value.trim();
    const body = document.getElementById('compose-body').value.trim();
    if (!toVal || !body) { showToast(t('letter_fill')||'Заполните получателя и текст'); return; }
    // Resolve recipient — prefer selected uid from dropdown, fallback to name/id match
    let uid = toInput.dataset.selectedUid || '';
    if (!uid) {
      const byName = contacts.find(c => c.name.toLowerCase() === toVal.toLowerCase());
      if (byName) uid = byName.user_id;
      else {
        const byUID = contacts.find(c => c.user_id === toVal);
        if (byUID) uid = byUID.user_id;
      }
    }
    if (!uid) { showToast(t('letter_unknown')||'Контакт не найден'); return; }
    try {
      const r = await fetch(`/api/letters/${uid}`, {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({subject, body})});
      if (r.ok) {
        document.getElementById('compose-view').classList.remove('open');
        showToast(t('letter_sent')||'Письмо отправлено');
        // Add to local letters list
        const d = await r.json();
        letters.unshift({id:d.id||'', from:window._identity?.userID, to:uid, fromName:'Вы', subject, body, timestamp:Math.floor(Date.now()/1000), outgoing:true, status:d.status});
        renderMailList();
      } else {
        const err = await r.text();
        showToast(err || 'Ошибка отправки');
      }
    } catch(e) { showToast('Ошибка сети'); }
  }

  // === CHAT VIEW ===
  function openChat(contact) {
    currentContact = contact; currentGroup = null; currentChannel = null;
    lastMsgJSON = ''; replyingTo = null;
    const rp = document.getElementById('reply-preview'); if(rp) rp.style.display='none';
    document.getElementById('chat-contact-name').textContent = contact.name;
    document.getElementById('btn-delete-chat').style.display = 'inline-flex';
    document.getElementById('btn-rename-contact').style.display = 'inline-flex';
    document.getElementById('chat-view').classList.add('open');
    if(msgCache[contact.user_id]) renderMessages(msgCache[contact.user_id]);
    else document.getElementById('messages').innerHTML = `<div class="empty-state"><p>${t('msg_loading')}</p></div>`;
    markAsRead(contact.user_id);
    loadMessages();
    document.getElementById('msg-input').focus();
  }

  function openGroupChat(group) {
    currentContact = null; currentGroup = group; currentChannel = null;
    lastGroupMsgJSON = ''; replyingTo = null;
    document.getElementById('chat-contact-name').textContent = group.name;
    document.getElementById('btn-delete-chat').style.display = 'inline-flex';
    document.getElementById('btn-rename-contact').style.display = 'none';
    document.getElementById('chat-view').classList.add('open');
    if(groupMsgCache[group.id]) renderGroupMessages(groupMsgCache[group.id]);
    else document.getElementById('messages').innerHTML = `<div class="empty-state"><p>${t('msg_loading')}</p></div>`;
    markAsRead('g:'+group.id);
    loadGroupMessages();
    document.getElementById('msg-input').focus();
  }

  function closeChat() {
    document.getElementById('chat-view').classList.remove('open');
    currentContact = null; currentGroup = null; currentChannel = null;
    renderChatsList();
  }

  // === MESSAGES ===
  async function loadMessages() {
    if(!currentContact) return;
    try {
      const r = await fetch(`/api/messages/${currentContact.user_id}`); const msgs = await r.json();
      const json = JSON.stringify(msgs);
      msgCache[currentContact.user_id] = msgs;
      if(msgs&&msgs.length>0) lastActivity[currentContact.user_id]=msgs[msgs.length-1].timestamp;
      if(json!==lastMsgJSON){lastMsgJSON=json;renderMessages(msgs);if(msgs&&msgs.length>0)markAsRead(currentContact.user_id);}
    }catch(e){}
  }

  const lockSVG = '<span class="msg-lock"><svg viewBox="0 0 24 24" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0110 0v4"/></svg></span>';

  function renderMessages(msgs) {
    const container = document.getElementById('messages');
    if(!msgs||msgs.length===0){container.innerHTML=`<div class="empty-state"><p>${t('msg_empty')}</p></div>`;return;}
    const isMasterChat = currentContact && isSpecialContact(currentContact.user_id);
    container.innerHTML = msgs.map(m => {
      const cls = m.outgoing?'out':'in';
      const masterCls = (isMasterChat&&!m.outgoing)?' master-msg':'';
      const dt = formatDateTime(m.timestamp);
      let check = '';
      if(m.outgoing) check = m.status==='delivered'?'<span class="check">✓✓</span>':'<span class="check">✓</span>';
      return `<div class="message ${cls}${masterCls}"><div class="msg-datetime">${dt}</div><div class="msg-text">${esc(m.text)}</div><div class="meta">${lockSVG}${check}</div></div>`;
    }).join('');
    const near = container.scrollHeight - container.scrollTop - container.clientHeight < 150;
    if(near) container.scrollTop = container.scrollHeight;
  }

  // === GROUP MESSAGES ===
  async function loadGroupMessages() {
    if(!currentGroup) return;
    try {
      const r = await fetch(`/api/groups/messages/${currentGroup.id}`); const msgs = await r.json();
      const json = JSON.stringify(msgs);
      groupMsgCache[currentGroup.id] = msgs;
      if(msgs&&msgs.length>0) lastActivity['g:'+currentGroup.id]=msgs[msgs.length-1].timestamp;
      if(json!==lastGroupMsgJSON){lastGroupMsgJSON=json;renderGroupMessages(msgs);if(msgs&&msgs.length>0)markAsRead('g:'+currentGroup.id);}
    }catch(e){}
  }

  function renderGroupMessages(msgs) {
    const container = document.getElementById('messages');
    if(!msgs||msgs.length===0){container.innerHTML=`<div class="empty-state"><p>${t('msg_empty_group')}</p></div>`;return;}
    container.innerHTML = msgs.map((m,idx) => {
      const cls=m.outgoing?'out':'in', dt=formatDateTime(m.timestamp);
      const sender = m.outgoing?'':`<div class="group-sender" style="color:${getAvatarColor(m.from_name)}">${esc(m.from_name)}</div>`;
      let replyBlock='';
      if(m.reply_to){const pt=m.reply_text?(m.reply_text.length>60?m.reply_text.substring(0,60)+'...':m.reply_text):'';replyBlock=`<div class="message-reply-quote" data-reply-id="${m.reply_to}"><div class="reply-quote-from">${esc(m.reply_from||'')}</div><div class="reply-quote-text">${esc(pt)}</div></div>`;}
      return `<div class="message ${cls}" data-msg-idx="${idx}">${sender}<div class="msg-datetime">${dt}</div>${replyBlock}<div class="msg-text">${esc(m.text)}</div><div class="meta">${lockSVG}</div></div>`;
    }).join('');
    // Reply handlers
    container.querySelectorAll('.message.in').forEach(el=>{
      el.addEventListener('click',e=>{if(e.target.closest('.message-reply-quote'))return;const idx=parseInt(el.dataset.msgIdx);if(msgs[idx])setReplyingTo(msgs[idx]);});
    });
    const near = container.scrollHeight - container.scrollTop - container.clientHeight < 150;
    if(near) container.scrollTop = container.scrollHeight;
  }

  function setReplyingTo(msg) {
    replyingTo = msg;
    let preview = document.getElementById('reply-preview');
    if(!preview){preview=document.createElement('div');preview.id='reply-preview';preview.className='reply-preview';document.getElementById('input-area').parentNode.insertBefore(preview,document.getElementById('input-area'));}
    const text=msg.text.length>80?msg.text.substring(0,80)+'...':msg.text;
    const sender=msg.outgoing?'Вы':(msg.from_name||msg.from);
    preview.innerHTML=`<div class="reply-preview-content"><div class="reply-preview-sender">${esc(sender)}</div><div class="reply-preview-text">${esc(text)}</div></div><button class="reply-preview-cancel" onclick="window._cancelReply()">&times;</button>`;
    preview.style.display='flex';
    document.getElementById('msg-input').focus();
  }
  window._cancelReply = function(){replyingTo=null;const p=document.getElementById('reply-preview');if(p)p.style.display='none';};

  // === SEND ===
  async function sendMessage() {
    if(!currentContact||_sending) return;
    const input=document.getElementById('msg-input'), text=input.value.trim();
    if(!text) return;
    _sending=true; input.value=''; input.style.height='auto';
    try{await fetch(`/api/messages/${currentContact.user_id}`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({text})});loadMessages();}catch(e){}finally{_sending=false;}
  }

  async function sendGroupMessage() {
    if(!currentGroup||_sending) return;
    const input=document.getElementById('msg-input'), text=input.value.trim();
    if(!text) return;
    _sending=true; input.value=''; input.style.height='auto';
    const body={text};
    if(replyingTo){body.replyTo=replyingTo.id;body.replyText=replyingTo.text.length>100?replyingTo.text.substring(0,100):replyingTo.text;body.replyFrom=replyingTo.outgoing?'Вы':(replyingTo.from_name||replyingTo.from);replyingTo=null;const p=document.getElementById('reply-preview');if(p)p.style.display='none';}
    try{await fetch(`/api/groups/messages/${currentGroup.id}`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});loadGroupMessages();}catch(e){}finally{_sending=false;}
  }

  // === UPDATE CHECK (FOTA) ===
  async function checkForUpdate() {
    try {
      const r=await fetch('/api/update/check'); const d=await r.json();
      if(!d.available) return;
      const dismissed=localStorage.getItem('iskra-update-dismissed');
      const key=d.remoteBuild?d.version+'-b'+d.remoteBuild:d.version;
      if(dismissed===key) return;
      // Find platform asset
      const ua=navigator.userAgent.toLowerCase();
      const isAndroid=ua.indexOf('android')!==-1;
      const isWindows=ua.indexOf('win')!==-1&&ua.indexOf('android')===-1;
      let asset=null;
      if(d.assets&&d.assets.length>0){
        if(isAndroid) asset=d.assets.find(a=>a.name.toLowerCase().endsWith('.apk'));
        else if(isWindows) asset=d.assets.find(a=>a.name.toLowerCase().endsWith('.exe'));
      }
      if(!asset) return;
      // Show update banner
      const banner=document.getElementById('update-banner');
      banner.innerHTML=`<div style="padding:10px 16px;background:var(--accent-light);display:flex;justify-content:space-between;align-items:center"><span style="font-size:13px">${t('update_available')}: ${esc(d.version)}</span><button onclick="localStorage.setItem('iskra-update-dismissed','${key}');this.parentElement.parentElement.style.display='none'" style="background:none;border:none;font-size:18px;cursor:pointer">&times;</button></div>`;
      banner.style.display='block';
    }catch(e){}
  }

  // === UNREAD ===
  function getLastRead(key){return parseInt(localStorage.getItem('iskra-lastread-'+key)||'0',10);}
  function markAsRead(key){
    let maxTs=0;
    if(key.startsWith('g:')){const gid=key.substring(2);const msgs=groupMsgCache[gid];if(msgs&&msgs.length>0)for(const m of msgs)if(m.timestamp>maxTs)maxTs=m.timestamp;}
    else{const msgs=msgCache[key];if(msgs&&msgs.length>0)for(const m of msgs)if(m.timestamp>maxTs)maxTs=m.timestamp;}
    if(maxTs===0)maxTs=Math.floor(Date.now()/1000);
    const prev=getLastRead(key);if(maxTs>prev)localStorage.setItem('iskra-lastread-'+key,maxTs.toString());
    unreadCounts[key]=0;
  }

  async function updateUnreadCounts() {
    const lastRead={};
    for(const c of contacts) lastRead[c.user_id]=getLastRead(c.user_id);
    for(const g of groups) lastRead['g:'+g.id]=getLastRead('g:'+g.id);
    try {
      const r=await fetch('/api/unread',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({lastRead})});
      const d=await r.json();
      unreadCounts=d.counts||{};
      lastMessages=d.lastMsg||{};
      if(d.lastTs) Object.assign(lastActivity,d.lastTs);
      const total=Object.values(unreadCounts).reduce((a,b)=>a+b,0);
      if(total>prevTotalUnread&&prevTotalUnread>=0) playPing();
      prevTotalUnread=total;
      // Update badges
      const chatBadge=document.getElementById('badge-chats');
      if(total>0){chatBadge.textContent=total;chatBadge.style.display='flex';}else{chatBadge.style.display='none';}
      if(currentTab==='chats') renderChatsList();
    }catch(e){}
  }

  function playPing(){try{const c=new(window.AudioContext||window.webkitAudioContext)(),o=c.createOscillator(),g=c.createGain();o.connect(g);g.connect(c.destination);o.frequency.value=880;o.type='sine';g.gain.setValueAtTime(0.3,c.currentTime);g.gain.exponentialRampToValueAtTime(0.001,c.currentTime+0.3);o.start(c.currentTime);o.stop(c.currentTime+0.3);}catch(e){}}

  // === POLLING ===
  function startPolling() {
    setInterval(()=>{if(currentContact)loadMessages();if(currentGroup)loadGroupMessages();},1000);
    setInterval(()=>{updateUnreadCounts();},1500);
    setInterval(()=>{loadContacts().then(()=>loadGroups()).then(()=>loadChannels());loadStatus();loadOnline();},10000);
  }

  // === EVENTS ===
  function setupEvents() {
    // Send
    document.getElementById('btn-send').addEventListener('click',()=>{if(currentGroup)sendGroupMessage();else sendMessage();});
    document.getElementById('msg-input').addEventListener('keydown',e=>{if(e.key==='Enter'&&!e.shiftKey){e.preventDefault();if(currentGroup)sendGroupMessage();else sendMessage();}});
    document.getElementById('msg-input').addEventListener('input',function(){this.style.height='auto';this.style.height=Math.min(this.scrollHeight,120)+'px';});

    // Back
    document.getElementById('btn-back').addEventListener('click',closeChat);

    // File attach
    document.getElementById('btn-attach').addEventListener('click',()=>document.getElementById('file-input').click());
    document.getElementById('file-input').addEventListener('change',async e=>{
      const file=e.target.files[0];if(!file)return;
      if(file.size>10*1024*1024){alert('Max 10 MB');return;}
      const uid=currentContact?currentContact.user_id:null;if(!uid)return;
      const form=new FormData();form.append('file',file);
      try{await fetch('/api/file/send/'+uid,{method:'POST',body:form});loadMessages();}catch(e){}
      e.target.value='';
    });

    // Contact list clicks (delegation)
    document.getElementById('contacts-list').addEventListener('click',e=>{
      const el=e.target.closest('.contact-item');
      if(el){showContextMenu(e,el.dataset.uid);e.preventDefault();}
    });

    // Chat list clicks
    document.getElementById('chats-list').addEventListener('click',e=>{
      const dm=e.target.closest('[data-uid]');
      if(dm){const c=contacts.find(x=>x.user_id===dm.dataset.uid)||{user_id:dm.dataset.uid,name:dm.dataset.uid.substring(0,8)};openChat(c);return;}
      const gp=e.target.closest('[data-gid]');
      if(gp){const g=groups.find(x=>x.id===gp.dataset.gid);if(g)openGroupChat(g);return;}
    });

    // Online list clicks
    document.getElementById('online-list').addEventListener('click',e=>{
      const el=e.target.closest('.contact-item');
      if(el) showContextMenu(e,el.dataset.uid);
    });

    // Context menu actions
    document.getElementById('context-menu').addEventListener('click',e=>{
      const item=e.target.closest('.ctx-item');
      if(!item) return;
      const action=item.dataset.action;
      const uid=document.getElementById('context-menu').dataset.uid;
      hideContextMenu();
      const contact=contacts.find(c=>c.user_id===uid)||{user_id:uid,name:uid.substring(0,8)};
      if(action==='message'){switchTab('chats');openChat(contact);}
      else if(action==='letter'){switchTab('mail');openCompose(contact.name||contact.user_id);}
      else if(action==='copy'){copyToClipboard(contact);}
      else if(action==='qr'){showContactQR(contact);}
      else if(action==='forward'){copyToClipboard(contact);}
      else if(action==='rename'){const n=prompt(t('rename_prompt'),contact.name);if(n&&n!==contact.name)fetch(`/api/contacts/rename/${uid}`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:n})}).then(()=>loadContacts());}
      else if(action==='delete'){if(confirm(`${t('delete_chat_confirm')} ${contact.name}?`))fetch(`/api/chat/delete/${uid}`,{method:'POST'}).then(()=>loadContacts());}
    });

    // Close context menu on click outside
    document.addEventListener('click',e=>{if(!e.target.closest('.context-menu')&&!e.target.closest('.contact-item'))hideContextMenu();});

    // Delete chat
    document.getElementById('btn-delete-chat').addEventListener('click',async()=>{
      if(currentGroup){if(!confirm(`${t('delete_group_confirm')} «${currentGroup.name}»?`))return;await fetch(`/api/groups/delete/${currentGroup.id}`,{method:'POST'});closeChat();loadGroups();}
      else if(currentContact){if(!confirm(`${t('delete_chat_confirm')} ${currentContact.name}?`))return;await fetch(`/api/chat/delete/${currentContact.user_id}`,{method:'POST'});closeChat();}
    });

    // Rename contact
    document.getElementById('btn-rename-contact').addEventListener('click',()=>{
      if(!currentContact) return;
      const n=prompt(t('rename_prompt'),currentContact.name);
      if(!n||n===currentContact.name) return;
      fetch(`/api/contacts/rename/${currentContact.user_id}`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:n})}).then(()=>{currentContact.name=n;document.getElementById('chat-contact-name').textContent=n;loadContacts();});
    });

    // Add contact
    document.getElementById('btn-add-contact').addEventListener('click',()=>{document.getElementById('modal-add').style.display='flex';document.getElementById('add-invite').focus();});

    // Auto-parse invite link
    document.getElementById('add-invite').addEventListener('input',function(){
      const parsed=parseInviteLink(this.value);
      if(parsed){document.getElementById('add-pubkey').value=parsed.pubkey;document.getElementById('add-x25519').value=parsed.x25519;if(parsed.name)document.getElementById('add-name').value=parsed.name;if(!document.getElementById('add-name').value)document.getElementById('add-name').focus();}
    });

    // Save contact
    document.getElementById('btn-add-save').addEventListener('click',async()=>{
      const inv=document.getElementById('add-invite').value.trim();
      if(inv){const p=parseInviteLink(inv);if(p){document.getElementById('add-pubkey').value=p.pubkey;document.getElementById('add-x25519').value=p.x25519;if(p.name&&!document.getElementById('add-name').value)document.getElementById('add-name').value=p.name;}}
      const name=document.getElementById('add-name').value.trim();
      const pubkey=document.getElementById('add-pubkey').value.trim();
      const x25519=document.getElementById('add-x25519').value.trim();
      if(!name||!pubkey) return;
      try{const r=await fetch('/api/contacts',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name,pubkeyBase58:pubkey,x25519Base58:x25519})});if(r.ok||r.status===201){closeModal('modal-add');clearAddForm();await loadContacts();}}catch(e){}
    });

    // Create group
    document.getElementById('btn-create-group').addEventListener('click',()=>{
      const ml=document.getElementById('group-members-list');
      if(!contacts||contacts.length===0){ml.innerHTML=`<p style="color:var(--text3)">${t('modal_group_no_contacts')}</p>`;}
      else{ml.innerHTML=contacts.map(c=>`<label class="group-member-option"><input type="checkbox" value="${c.user_id}"/><span>${esc(c.name)}</span></label>`).join('');}
      document.getElementById('group-name').value='';
      document.getElementById('modal-group').style.display='flex';
    });

    document.getElementById('btn-group-create').addEventListener('click',async()=>{
      const name=document.getElementById('group-name').value.trim();if(!name)return;
      const members=Array.from(document.querySelectorAll('#group-members-list input:checked')).map(el=>el.value);if(members.length===0)return;
      try{const r=await fetch('/api/groups',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name,members})});if(r.ok){closeModal('modal-group');await loadGroups();}}catch(e){}
    });

    // Create channel
    document.getElementById('btn-create-channel').addEventListener('click',async()=>{
      const title=prompt(t('channel_name_prompt')||'Channel name:');if(!title)return;
      try{await fetch('/api/channels/create',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({title})});loadChannels();}catch(e){}
    });

    // Restore
    document.getElementById('btn-restore-go').addEventListener('click',async()=>{
      const words=document.getElementById('restore-words').value.trim();const err=document.getElementById('restore-error');err.style.display='none';
      if(!words){err.textContent=t('restore_enter_words');err.style.display='block';return;}
      try{const r=await fetch('/api/restore',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({words})});const d=await r.json();if(d.error){err.textContent=d.error;err.style.display='block';}else{localStorage.setItem('iskra-started','1');document.getElementById('restore-words').value='';alert(t('restore_success')+' ID: '+d.userID);closeModal('modal-restore');}}catch(e){err.textContent=t('restore_error');err.style.display='block';}
    });

    // Settings
    document.getElementById('btn-settings').addEventListener('click',()=>{document.getElementById('settings-view').classList.add('open');});
    document.getElementById('btn-settings-back').addEventListener('click',()=>{document.getElementById('settings-view').classList.remove('open');});

    // Theme toggle
    document.getElementById('btn-theme').addEventListener('click',()=>{
      const cur=localStorage.getItem('iskra-theme')||'light';
      const next=cur==='light'?'dark':'light';
      localStorage.setItem('iskra-theme',next);
      applyTheme();
      const sel=document.getElementById('settings-theme');if(sel)sel.value=next;
    });
    document.getElementById('settings-theme').addEventListener('change',function(){
      localStorage.setItem('iskra-theme',this.value);applyTheme();
    });

    // PIN change
    document.getElementById('btn-change-pin').addEventListener('click',()=>{
      pinMode='setup';pinValue='';pinSetupFirst='';updatePINDots();
      document.getElementById('pin-subtitle').textContent=t('pin_setup');
      document.getElementById('pin-ok').textContent=t('pin_btn_save');
      document.getElementById('pin-error').textContent='';document.getElementById('pin-attempts').textContent='';
      document.getElementById('pin-screen').style.display='flex';
    });

    // Help
    document.getElementById('btn-help').addEventListener('click',()=>{document.getElementById('modal-help').style.display='flex';});

    // QR
    document.getElementById('btn-show-qr').addEventListener('click',()=>{
      const id=window._identity;if(!id)return;
      document.getElementById('qr-link').textContent=makeInviteLink(id);
      document.getElementById('qr-short-addr').textContent='ID: '+id.userID;
      document.getElementById('modal-qr').style.display='flex';
      document.getElementById('settings-view').classList.remove('open');
    });
    document.getElementById('btn-copy-qr').addEventListener('click',()=>{
      const link=document.getElementById('qr-link').textContent;
      navigator.clipboard.writeText(link).then(()=>{document.getElementById('btn-copy-qr').textContent=t('btn_copied');setTimeout(()=>document.getElementById('btn-copy-qr').textContent=t('qr_copy'),2000);});
    });

    // Mail: compose button + dropdown
    setupComposeDropdown();
    document.getElementById('btn-compose').addEventListener('click',()=>openCompose(''));
    // Mail: send letter
    document.getElementById('btn-compose-send').addEventListener('click',sendLetter);
    document.getElementById('btn-compose-back').addEventListener('click',()=>document.getElementById('compose-view').classList.remove('open'));
    // Mail: letter back, reply, forward
    document.getElementById('btn-letter-back').addEventListener('click',()=>document.getElementById('letter-view').classList.remove('open'));
    document.getElementById('btn-letter-reply').addEventListener('click',()=>{
      const subj = document.getElementById('letter-subject').textContent || '';
      const from = document.getElementById('letter-view').dataset.fromName || '';
      document.getElementById('letter-view').classList.remove('open');
      openCompose(from);
      document.getElementById('compose-subject').value = subj.startsWith('Re:') ? subj : 'Re: ' + subj;
    });
    document.getElementById('btn-letter-forward').addEventListener('click',()=>{
      const subj = document.getElementById('letter-subject').textContent || '';
      const body = document.getElementById('letter-body').textContent || '';
      document.getElementById('letter-view').classList.remove('open');
      openCompose('');
      document.getElementById('compose-subject').value = 'Fwd: ' + subj;
      document.getElementById('compose-body').value = '\n\n--- Пересланное письмо ---\n' + body;
    });
    // Mail: letter list clicks (delegation on all three folders)
    ['mail-inbox-list','mail-sent-list','mail-delivered-list'].forEach(id=>{
      const container=document.getElementById(id);
      if(container) container.addEventListener('click',e=>{
        const el=e.target.closest('.letter-item');
        if(el){const l=letters.find(x=>x.id===el.dataset.lid);if(l)openLetter(l);}
      });
    });
    // Mail: context menu "letter" action → open compose with recipient
    // (handled inline in context menu handler above)

    // Close modals
    document.querySelectorAll('.modal').forEach(m=>{m.addEventListener('click',e=>{if(e.target===m)m.style.display='none';});});
    document.addEventListener('keydown',e=>{if(e.key==='Escape')document.querySelectorAll('.modal').forEach(m=>m.style.display='none');});

    // Scroll-to-bottom FAB
    const messagesEl=document.getElementById('messages'),scrollBtn=document.getElementById('scroll-bottom');
    if(messagesEl&&scrollBtn){
      messagesEl.addEventListener('scroll',()=>{scrollBtn.style.display=(messagesEl.scrollHeight-messagesEl.scrollTop-messagesEl.clientHeight>200)?'flex':'none';});
      scrollBtn.addEventListener('click',()=>{messagesEl.scrollTop=messagesEl.scrollHeight;scrollBtn.style.display='none';});
    }
  }

  // === CLIPBOARD & QR HELPERS ===
  function copyToClipboard(contact) {
    const id = window._identity;
    if (!id) return;
    const link = `iskra://${contact.pubkey || contact.user_id}`;
    // Try modern API first, fallback to textarea trick
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(link).then(() => {
        showToast(t('copied') || 'Скопировано');
      }).catch(() => fallbackCopy(link));
    } else {
      fallbackCopy(link);
    }
  }

  function fallbackCopy(text) {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.cssText = 'position:fixed;left:-9999px;top:-9999px';
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand('copy'); showToast(t('copied') || 'Скопировано'); } catch(e) { showToast('Copy failed'); }
    document.body.removeChild(ta);
  }

  function showToast(msg) {
    let toast = document.getElementById('toast');
    if (!toast) { toast = document.createElement('div'); toast.id = 'toast'; toast.style.cssText = 'position:fixed;bottom:80px;left:50%;transform:translateX(-50%);background:rgba(0,0,0,0.8);color:#fff;padding:8px 20px;border-radius:20px;font-size:13px;z-index:999;transition:opacity 0.3s'; document.body.appendChild(toast); }
    toast.textContent = msg; toast.style.opacity = '1';
    setTimeout(() => { toast.style.opacity = '0'; }, 2000);
  }

  function showContactQR(contact) {
    // Reuse the QR modal but show contact's info
    const link = `iskra://${contact.pubkey || contact.user_id}`;
    document.getElementById('qr-link').textContent = link;
    document.getElementById('qr-short-addr').textContent = contact.name || '';
    document.getElementById('qr-code').innerHTML = `<div style="padding:20px;text-align:center;font-size:13px;color:var(--text2)">${esc(link)}</div>`;
    document.getElementById('modal-qr').style.display = 'flex';
  }

  // === CONTEXT MENU ===
  function showContextMenu(e, uid) {
    e.stopPropagation();
    const menu = document.getElementById('context-menu');
    menu.dataset.uid = uid;
    const rect = e.target.closest('.contact-item').getBoundingClientRect();
    menu.style.top = Math.min(rect.bottom, window.innerHeight - 300) + 'px';
    menu.style.left = Math.min(rect.left + 20, window.innerWidth - 220) + 'px';
    menu.style.display = 'block';
  }

  function hideContextMenu() { document.getElementById('context-menu').style.display = 'none'; }

  // === INVITE LINK PARSING ===
  function parseInviteLink(link) {
    link = link.trim();
    const m = link.match(/^iskra:\/\/([A-Za-z0-9]+)\/([A-Za-z0-9]+)(?:\/(.+))?$/);
    if(!m) return null;
    return {pubkey:m[1],x25519:m[2],name:m[3]?decodeURIComponent(m[3]):''};
  }

  // === UTILS ===
  function esc(s){const d=document.createElement('div');d.textContent=s;return d.innerHTML;}
  function clearAddForm(){['add-invite','add-name','add-pubkey','add-x25519'].forEach(id=>document.getElementById(id).value='');}

  function formatDateTime(ts) {
    const d=new Date(ts*1000),now=new Date(),time=d.toLocaleTimeString('ru-RU',{hour:'2-digit',minute:'2-digit'});
    const yesterday=new Date(now);yesterday.setDate(yesterday.getDate()-1);
    if(d.toDateString()===now.toDateString()) return `${t('time_today')}, ${time}`;
    if(d.toDateString()===yesterday.toDateString()) return `${t('time_yesterday')}, ${time}`;
    return `${d.toLocaleDateString('ru-RU',{day:'numeric',month:'long'})}, ${time}`;
  }

  function formatContactTime(ts) {
    if(!ts) return '';
    const d=new Date(ts*1000),now=new Date(),time=d.toLocaleTimeString('ru-RU',{hour:'2-digit',minute:'2-digit'});
    const yesterday=new Date(now);yesterday.setDate(yesterday.getDate()-1);
    if(d.toDateString()===now.toDateString()) return time;
    if(d.toDateString()===yesterday.toDateString()) return t('time_yesterday');
    if(d.getFullYear()===now.getFullYear()) return d.toLocaleDateString('ru-RU',{day:'numeric',month:'short'});
    return d.toLocaleDateString('ru-RU',{day:'2-digit',month:'2-digit',year:'2-digit'});
  }

  window.closeModal = function(id){document.getElementById(id).style.display='none';};

  // Hardware back (Android)
  window._handleBack = function(){
    const openModal=document.querySelector('.modal[style*="display: flex"],.modal[style*="display:flex"]');
    if(openModal){openModal.style.display='none';return true;}
    if(document.getElementById('settings-view').classList.contains('open')){document.getElementById('settings-view').classList.remove('open');return true;}
    if(document.getElementById('chat-view').classList.contains('open')){closeChat();return true;}
    return false;
  };

  document.addEventListener('DOMContentLoaded', init);
})();

const ICONS = {
  dashboard: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="3" width="7" height="9" rx="1"/><rect x="14" y="3" width="7" height="5" rx="1"/><rect x="14" y="12" width="7" height="9" rx="1"/><rect x="3" y="16" width="7" height="5" rx="1"/></svg>',
  projects: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 7h7l2 2h9v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z"/><path d="M3 7V5a2 2 0 0 1 2-2h4l2 2"/></svg>',
  settings: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3h.1a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9v.1a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg>',
  users: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>',
  file: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" class="file-icon"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/></svg>',
  folder: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" class="file-icon"><path d="M3 7h7l2 2h9v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z"/></svg>',
  download: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="M7 10l5 5 5-5"/><path d="M12 15V3"/></svg>',
  upload: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="M17 8l-5-5-5 5"/><path d="M12 3v12"/></svg>',
  shield: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>',
  globe: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>',
  logout: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5"/><path d="M21 12H9"/></svg>',
  refresh: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M23 4v6h-6"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>',
  trash: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',
  copy: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>',
  external: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><path d="M15 3h6v6"/><path d="M10 14L21 3"/></svg>',
  back: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M19 12H5"/><path d="M12 19l-7-7 7-7"/></svg>',
};

function h(tag, attrs = {}, children = []) {
  const node = document.createElement(tag);
  for (const [key, val] of Object.entries(attrs)) {
    if (val === null || val === undefined || val === false) continue;
    if (key === 'class') node.className = val;
    else if (key === 'html') node.innerHTML = val;
    else if (key === 'style') node.setAttribute('style', val);
    else if (key.startsWith('on') && typeof val === 'function') node.addEventListener(key.slice(2), val);
    else if (key === 'dataset') Object.assign(node.dataset, val);
    else node.setAttribute(key, val === true ? '' : val);
  }
  for (const child of [].concat(children)) {
    if (child === null || child === undefined || child === false) continue;
    if (typeof child === 'string' && child.trimStart().startsWith('<')) {
      node.insertAdjacentHTML('beforeend', child);
    } else {
      node.append(child.nodeType ? child : document.createTextNode(child));
    }
  }
  return node;
}

function toast(message, type = 'ok', timeout = 4000) {
  const root = document.getElementById('toast-root');
  // Текстом, а не HTML: в сообщениях об ошибках бывает ввод пользователя (домен и т.п.).
  const t = h('div', { class: 'toast ' + type }, String(message));
  root.appendChild(t);
  setTimeout(() => { t.style.opacity = '0'; t.style.transition = 'opacity 0.3s'; }, timeout);
  setTimeout(() => t.remove(), timeout + 350);
}

function openModal(title, bodyEl, footEl) {
  const root = document.getElementById('modal-root');
  root.innerHTML = '';
  const modal = h('div', { class: 'modal' });
  const head = h('div', { class: 'modal-head' }, [
    h('div', { class: 'modal-title', html: title }),
    h('button', { class: 'modal-close', html: '&times;', onclick: closeModal }),
  ]);
  modal.append(head);
  if (bodyEl) modal.append(h('div', { class: 'modal-body' }, [bodyEl]));
  if (footEl) modal.append(h('div', { class: 'modal-foot' }, footEl));
  const overlay = h('div', { class: 'modal-overlay', onclick: (e) => { if (e.target === overlay) closeModal(); } }, [modal]);
  root.appendChild(overlay);
  const firstInput = modal.querySelector('input,select,textarea');
  if (firstInput) setTimeout(() => firstInput.focus(), 50);
  return modal;
}

function closeModal() {
  document.getElementById('modal-root').innerHTML = '';
}

function confirmDialog(title, message, danger = false) {
  return new Promise((resolve) => {
    let resolved = false;
    const finish = (v) => { if (!resolved) { resolved = true; closeModal(); resolve(v); } };
    const body = h('div', { html: message });
    const foot = [
      h('button', { class: 'btn', onclick: () => finish(false) }, 'Отмена'),
      h('button', { class: 'btn ' + (danger ? 'btn-danger' : 'btn-primary'), onclick: () => finish(true) }, danger ? 'Удалить' : 'Подтвердить'),
    ];
    openModal(title, body, foot);
  });
}

const UNITS = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];

function formatBytes(n) {
  if (n == null || n === 0) return '0 Б';
  let v = n, i = 0;
  while (v >= 1024 && i < UNITS.length - 1) { v /= 1024; i++; }
  return (i === 0 ? String(Math.round(v)) : v.toFixed(1)) + ' ' + UNITS[i];
}

function formatDate(s) {
  if (!s) return '—';
  return s.replace('T', ' ').slice(0, 16);
}

function formatNumber(n) {
  return new Intl.NumberFormat('ru-RU').format(n || 0);
}

function escapeHtml(s) {
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function badge(status) {
  const map = {
    active: ['badge-ok', 'Активен'],
    error: ['badge-err', 'Ошибка'],
    pending: ['badge-warn', 'В обработке'],
    ok: ['badge-ok', 'Готово'],
    disabled: ['badge-neutral', 'Выключен'],
    on: ['badge-ok', 'Включён'],
    off: ['badge-neutral', 'Выключен'],
    ssl: ['badge-info', 'SSL'],
    user: ['badge-neutral', 'Пользователь'],
    admin: ['badge-info', 'Администратор'],
  };
  const m = map[status] || ['badge-neutral', status];
  return h('span', { class: 'badge ' + m[0], html: m[1] });
}

function progressBar(used, total) {
  const pct = total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0;
  const cls = pct >= 90 ? 'danger' : pct >= 70 ? 'warn' : '';
  return h('div', {}, [
    h('div', { class: 'progress' }, [h('div', { class: 'progress-bar ' + cls, style: 'width:' + pct + '%' })]),
    h('div', { class: 'progress-meta' }, [
      h('span', { html: formatBytes(used) }),
      h('span', { html: pct + '% из ' + formatBytes(total) }),
    ]),
  ]);
}

function chart(canvas, labels, series, options = {}) {
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.clientWidth || 400;
  const height = options.height || 180;
  canvas.style.height = height + 'px';
  canvas.width = w * dpr;
  canvas.height = height * dpr;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, height);

  const padL = options.padL || 48;
  const padR = 12, padT = 14, padB = 24;
  const plotW = w - padL - padR, plotH = height - padT - padB;
  if (plotW < 10 || plotH < 10) return;

  const colors = options.colors || ['#2F6FED', '#12B76A'];
  const fmt = options.format || ((v) => v.toFixed(1));

  let max = 0;
  for (const s of series) for (const v of s) if (v > max) max = v;
  if (max === 0) max = 1;

  ctx.font = '10px IBM Plex Sans, sans-serif';
  ctx.fillStyle = '#667085';
  const steps = 4;
  for (let i = 0; i <= steps; i++) {
    const y = padT + plotH - (plotH * i) / steps;
    ctx.strokeStyle = '#EDF0F4';
    ctx.beginPath();
    ctx.moveTo(padL, y);
    ctx.lineTo(w - padR, y);
    ctx.stroke();
    const val = (max * i) / steps;
    ctx.textAlign = 'right';
    ctx.fillText(fmt(val), padL - 6, y + 3);
  }

  series.forEach((data, si) => {
    if (!data || data.length === 0) return;
    const color = colors[si % colors.length];
    ctx.strokeStyle = color;
    ctx.lineWidth = 2;
    ctx.beginPath();
    const n = data.length;
    data.forEach((v, i) => {
      const x = padL + (n === 1 ? plotW / 2 : (plotW * i) / (n - 1));
      const y = padT + plotH - (plotH * v) / max;
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();
  });

  if (labels && labels.length > 1) {
    ctx.fillStyle = '#98A2B3';
    ctx.font = '10px IBM Plex Sans, sans-serif';
    const n = labels.length;
    const show = Math.min(6, n);
    for (let i = 0; i < show; i++) {
      const idx = Math.round((i * (n - 1)) / Math.max(1, show - 1));
      const x = padL + (plotW * idx) / (n - 1);
      ctx.textAlign = 'center';
      ctx.fillText(labels[idx], x, height - 8);
    }
  }
}

function skeleton() {
  return h('div', {}, [h('div', { class: 'spinner' })]);
}

function breadcrumbs(items) {
  const parts = [];
  items.forEach((it, i) => {
    if (i > 0) parts.push(h('span', { class: 'sep', html: '/' }));
    parts.push(it.href
      ? h('a', { href: it.href, html: escapeHtml(it.label) })
      : h('span', { class: 'current', html: escapeHtml(it.label) }));
  });
  return h('div', { class: 'breadcrumbs' }, parts);
}
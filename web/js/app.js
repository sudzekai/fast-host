const state = {
  me: null,
  used: 0,
  projectCount: 0,
  view: null,
	usageStatus: null,
};

function applyTheme(theme) {
  document.documentElement.setAttribute('data-theme', theme);
  try { localStorage.setItem('fasthost-theme', theme); } catch (e) {}
}

function initialTheme() {
  try {
    const saved = localStorage.getItem('fasthost-theme');
    if (saved === 'light' || saved === 'dark') return saved;
  } catch (e) {}
  return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function toggleTheme() {
  applyTheme(document.documentElement.getAttribute('data-theme') === 'dark' ? 'light' : 'dark');
}

function route() {
  const hash = (location.hash || '#/').replace(/^#/, '');
  return hash.split('/').filter(Boolean);
}

async function boot() {
  applyTheme(initialTheme());
  window.addEventListener('hashchange', () => render());
  // Квоту/роль мог поменять администратор, пока вкладка была открыта.
  document.addEventListener('visibilitychange', async () => {
    if (document.visibilityState !== 'visible' || !state.me) return;
    const before = JSON.stringify(state.me);
    await refreshMe();
    if (JSON.stringify(state.me) !== before) render({ skipRefresh: true });
  });
  await refreshMe();
  render({ skipRefresh: true });
}

async function refreshMe(silent = false) {
  try {
    const data = await api.get('/api/me', { silent });
    state.me = data.user;
    state.used = data.used_bytes || 0;
    state.projectCount = data.project_count || 0;
    state.usageStatus = data.usage_status;
    return true;
  } catch (e) {
    if (e.status === 401) state.me = null;
    return false;
  }
}

let renderSeq = 0;
let pagePoll;
function stopPagePolling() { clearInterval(pagePoll); pagePoll = null; }
function startPagePolling(page, action) {
  stopPagePolling();
  let busy = false;
  pagePoll = setInterval(async () => {
    if (busy || !page.isConnected || page.querySelector('[aria-busy="true"]') || document.visibilityState !== 'visible') return;
    busy = true;
    try { await action(); } catch (_) {} finally { busy = false; }
  }, 10000);
}

function storageNotice(status) {
  const s = status || {};
  const problem = s.pending_projects || s.stale_projects || s.error_projects;
  return h('div', { class: 'card card-pad mb-24' }, [
    h('div', { class: 'metric-status ' + (problem ? 'stale' : 'ok') }, [
      h('strong', {}, problem ? 'Часть размеров ожидает обновления' : 'Использование хранилища обновляется'),
      h('div', {}, 'Последний замер всех проектов: ' + localTimestamp(s.updated_at)),
      problem ? h('div', {}, 'Без замера: ' + (s.pending_projects || 0) + ' · Устарели: ' + (s.stale_projects || 0) + ' · Ошибки: ' + (s.error_projects || 0)) : null,
    ]),
    h('div', { class: 'hint mb-24' }, 'Квота: файлы сайта и данные PHP. Служебные логи и временные файлы загрузки не включены. Сбор каждые ' + (s.interval_seconds || 60) + ' с.'),
    h('button', { class: 'btn btn-sm', disabled: !s.can_refresh, onclick: async () => {
      await api.post('/api/metrics/refresh'); await render(); toast('Размеры файлов пересчитаны');
    } }, 'Обновить использование диска'),
  ]);
}

function renderMemory(target, memory) {
  if (!target.isConnected) return;
  if (!memory || !memory.available) {
    target.replaceChildren(h('div', { class: 'metric-status error' }, (memory && memory.error) || 'RAM сервера недоступна')); return;
  }
  target.replaceChildren(
    h('div', { class: 'metrics-summary' }, [
      infoItem('Используется RAM', formatBytes(memory.used_bytes)),
      infoItem('Доступно RAM', formatBytes(memory.available_bytes)),
      infoItem('Всего RAM', formatBytes(memory.total_bytes)),
      infoItem('Swap используется', formatBytes(memory.swap_used_bytes) + ' из ' + formatBytes(memory.swap_total_bytes)),
    ]), progressBar(memory.used_bytes, memory.total_bytes),
    h('div', { class: 'hint mt-16' }, 'RAM всего сервера, включая другие приложения. Доступная память учитывает освобождаемый системный кэш. Замер: ' + localTimestamp(memory.sampled_at)));
}

// Перед каждой отрисовкой панели перечитываем /api/me: квота, роль и занятое место
// всегда актуальны, а истёкшая сессия сразу ведёт на страницу входа.
async function render(opts = {}) {
  stopPagePolling();
  const seq = ++renderSeq;
  if (state.me && !opts.skipRefresh) {
    await refreshMe();
    if (seq !== renderSeq) return;
  }
  const app = document.getElementById('app');
  app.innerHTML = '';
  const parts = route();
  if (!state.me) {
    if (parts[0] === 'register') renderRegister(app);
    else renderLogin(app);
    return;
  }
  renderShell(app, parts);
}

/* ================= ЛОГИН / РЕГИСТРАЦИЯ ================= */

function renderLogin(app) {
  const card = h('div', { class: 'auth-card' }, [
    h('div', { class: 'auth-brand' }, [h('span', { class: 'brand-mark', html: 'F' }), h('span', {}, 'FastHost')]),
    h('div', { class: 'auth-sub', html: 'Панель управления хостингом' }),
    h('div', { class: 'field' }, [
      h('label', { html: 'Логин' }),
      h('input', { class: 'input', id: 'login-user', autocomplete: 'username' }),
    ]),
    h('div', { class: 'field' }, [
      h('label', { html: 'Пароль' }),
      h('input', { class: 'input', id: 'login-pass', type: 'password', autocomplete: 'current-password' }),
    ]),
    h('label', { class: 'checkbox-row' }, [
      h('input', { type: 'checkbox', id: 'login-remember' }),
      h('span', { html: 'Запомнить меня на 30 дней' }),
    ]),
    h('button', { class: 'btn btn-primary btn-block', id: 'login-btn' }, 'Войти'),
    h('div', { class: 'auth-switch' }, ['Нет аккаунта? ', h('a', { href: '#/register', html: 'Регистрация' })]),
  ]);
  app.append(h('div', { class: 'auth-wrap' }, [card]));

  const doLogin = async () => {
    const btn = document.getElementById('login-btn');
    return runAction(btn, async () => {
    try {
      await api.post('/api/auth/login', {
        username: document.getElementById('login-user').value.trim(),
        password: document.getElementById('login-pass').value,
        remember: document.getElementById('login-remember').checked,
      });
      await refreshMe();
      location.hash = '#/';
      render({ skipRefresh: true });
    } catch (e) {
      toast(e.message, 'err');
    }
    }, 'Входим…');
  };
  document.getElementById('login-btn').addEventListener('click', doLogin);
  document.getElementById('login-pass').addEventListener('keydown', (e) => { if (e.key === 'Enter') doLogin(); });
}

function renderRegister(app) {
  const card = h('div', { class: 'auth-card' }, [
    h('div', { class: 'auth-brand' }, [h('span', { class: 'brand-mark', html: 'F' }), h('span', {}, 'FastHost')]),
    h('div', { class: 'auth-sub', html: 'Регистрация клиента хостинга' }),
    h('div', { class: 'field' }, [
      h('label', { html: 'Логин' }),
      h('input', { class: 'input', id: 'reg-user', autocomplete: 'username' }),
      h('div', { class: 'hint', html: '3-32 символа: латиница, цифры, подчёркивание' }),
    ]),
    h('div', { class: 'field' }, [
      h('label', { html: 'Email' }),
      h('input', { class: 'input', id: 'reg-email', type: 'email', autocomplete: 'email' }),
    ]),
    h('div', { class: 'field' }, [
      h('label', { html: 'Пароль' }),
      h('input', { class: 'input', id: 'reg-pass', type: 'password', autocomplete: 'new-password' }),
      h('div', { class: 'hint', html: 'Не короче 6 символов' }),
    ]),
    h('button', { class: 'btn btn-primary btn-block', id: 'reg-btn' }, 'Создать аккаунт'),
    h('div', { class: 'auth-switch' }, ['Уже есть аккаунт? ', h('a', { href: '#/login', html: 'Войти' })]),
  ]);
  app.append(h('div', { class: 'auth-wrap' }, [card]));

  const doReg = async () => {
    const btn = document.getElementById('reg-btn');
    return runAction(btn, async () => {
    try {
      await api.post('/api/auth/register', {
        username: document.getElementById('reg-user').value.trim(),
        email: document.getElementById('reg-email').value.trim(),
        password: document.getElementById('reg-pass').value,
      });
      await refreshMe();
      location.hash = '#/';
      render({ skipRefresh: true });
    } catch (e) {
      toast(e.message, 'err');
    }
    }, 'Создаём аккаунт…');
  };
  document.getElementById('reg-btn').addEventListener('click', doReg);
  document.getElementById('reg-pass').addEventListener('keydown', (e) => { if (e.key === 'Enter') doReg(); });
}

/* ================= ОБОЛОЧКА ================= */

function renderShell(app, parts) {
  const menu = [
    { href: '#/', icon: ICONS.dashboard, label: 'Панель', key: '' },
    { href: '#/projects', icon: ICONS.projects, label: 'Проекты', key: 'projects' },
    { href: '#/settings', icon: ICONS.settings, label: 'Настройки', key: 'settings' },
  ];
  if (state.me && state.me.role === 'admin') {
    menu.push({ href: '#/admin/users', icon: ICONS.users, label: 'Пользователи', key: 'admin' });
  }
  const activeKey = parts[0] || '';

  const nav = h('nav', { class: 'sidebar' }, [
    h('div', { class: 'nav-section-label', html: 'Управление' }),
    ...menu.map((m) =>
      h('a', { class: 'nav-item' + (activeKey === m.key ? ' active' : ''), href: m.href }, [m.icon, m.label])
    ),
  ]);

  const logoutBtn = h('button', {
    class: 'btn btn-sm',
    onclick: async () => {
      await api.post('/api/auth/logout');
      state.me = null;
      location.hash = '#/';
      render({ skipRefresh: true });
    },
  }, ICONS.logout + ' Выйти');

  const themeBtn = h('button', {
    class: 'btn btn-sm',
    onclick: toggleTheme,
  }, 'Тема');

  const topbar = h('header', { class: 'topbar' }, [
    h('div', { class: 'brand' }, [h('span', { class: 'brand-mark', html: 'F' }), h('span', {}, 'FastHost')]),
    h('div', { class: 'topbar-spacer' }),
    themeBtn,
    h('div', { class: 'user-chip' }, [
      h('div', { class: 'avatar', html: (state.me.username || '?').slice(0, 2) }),
      h('div', { class: 'user-meta' }, [
        h('div', { class: 'name', html: escapeHtml(state.me.username) }),
        h('div', {}, badge(state.me.role)),
      ]),
      logoutBtn,
    ]),
  ]);

  const content = h('main', { class: 'content', id: 'page' });
  app.append(topbar);
  app.append(h('div', { class: 'layout' }, [nav, content]));

  const page = document.getElementById('page');
  page.innerHTML = '';
  page.append(skeleton());

  if (activeKey === 'projects') {
    if (parts[1]) renderProjectDetail(page, parts[1]);
    else renderProjectsList(page);
  } else if (activeKey === 'settings') {
    renderSettings(page);
  } else if (activeKey === 'admin' && state.me.role === 'admin') {
    if (parts[1] === 'users' && parts[2]) renderAdminUserDetail(page, parts[2]);
    else if (parts[1] === 'users') renderAdminUsers(page);
    else renderDashboard(page);
  } else {
    renderDashboard(page);
  }
}

/* ================= ДАШБОРД ================= */

async function renderDashboard(page) {
  page.innerHTML = '';
  page.append(h('div', { class: 'page-head' }, [
    h('div', {}, [
      h('div', { class: 'page-title', html: 'Панель' }),
      h('div', { class: 'page-sub', html: 'Обзор вашего хостинга. Здравствуйте, ' + escapeHtml(state.me.username) + '.' }),
    ]),
    h('div', { class: 'page-actions' }, [
      h('button', { class: 'btn btn-primary', onclick: () => { location.hash = '#/projects'; createProjectModal(); } }, ICONS.upload + ' Новый проект'),
    ]),
  ]));

  try {
    const data = await api.get('/api/projects');
    const quota = data.quota || 0;
    const pct = quota > 0 ? Math.min(100, Math.round((data.used_bytes / quota) * 100)) : 0;

    page.append(h('div', { class: 'grid grid-3 mb-24' }, [
      h('div', { class: 'card stat-card' }, [
        h('div', { class: 'stat-label', html: 'Проекты' }),
        h('div', { class: 'stat-value', html: String(data.projects.length) }),
        h('div', { class: 'stat-hint', html: 'включено: ' + data.projects.filter((p) => p.enabled).length }),
      ]),
      h('div', { class: 'card stat-card' }, [
        h('div', { class: 'stat-label', html: 'Занято на диске' }),
        h('div', { class: 'stat-value', id: 'dashboard-used', html: formatBytes(data.used_bytes) }),
        h('div', { class: 'mt-8', id: 'dashboard-progress' }, [progressBar(data.used_bytes, quota)]),
      ]),
      h('div', { class: 'card stat-card' }, [
        h('div', { class: 'stat-label', html: 'Квота хранения' }),
        h('div', { class: 'stat-value', id: 'dashboard-quota', html: formatBytes(quota) }),
        h('div', { class: 'stat-hint', id: 'dashboard-quota-hint', html: 'лимит: ' + pct + '% использовано' }),
      ]),
    ]));

    const usageNotice = h('div', {}, [storageNotice(data.usage_status)]);
    page.append(usageNotice);
    let memoryBody;
    if (state.me.role === 'admin') {
      memoryBody = h('div', { class: 'card-pad' }, [skeleton()]);
      page.append(h('div', { class: 'card mb-24' }, [
        h('div', { class: 'card-head' }, [h('div', { class: 'card-title' }, 'Оперативная память сервера (RAM)'),
          h('button', { class: 'btn btn-sm', onclick: async () => renderMemory(memoryBody, (await api.get('/api/admin/stats')).memory) }, 'Обновить RAM')]), memoryBody,
      ]));
      api.get('/api/admin/stats').then((stats) => renderMemory(memoryBody, stats.memory)).catch((error) => { if (memoryBody.isConnected) memoryBody.textContent = error.message; });
    }
    startPagePolling(page, async () => {
      const current = await api.get('/api/projects', { silent: true });
      if (!page.isConnected) return;
      document.getElementById('dashboard-used').textContent = formatBytes(current.used_bytes);
      document.getElementById('dashboard-progress').replaceChildren(progressBar(current.used_bytes, current.quota));
      document.getElementById('dashboard-quota').textContent = formatBytes(current.quota);
      document.getElementById('dashboard-quota-hint').textContent = 'лимит: ' + (current.quota > 0 ? Math.round(current.used_bytes / current.quota * 100) : 0) + '% использовано';
      for (const project of current.projects) {
        const size = page.querySelector('[data-project-size="' + project.id + '"]');
        if (size) size.textContent = formatBytes(project.disk_bytes);
      }
      usageNotice.replaceChildren(storageNotice(current.usage_status));
      if (memoryBody) {
        try { renderMemory(memoryBody, (await api.get('/api/admin/stats', { silent: true })).memory); }
        catch (error) { if (memoryBody.isConnected) memoryBody.prepend(h('div', { class: 'metric-status error' }, 'Не удалось обновить RAM: ' + error.message)); }
      }
    });

    if (data.projects.length === 0) {
      page.append(h('div', { class: 'card empty-state' }, [
        h('div', { class: 'big', html: '&#128193;' }),
        h('div', {}, 'Проектов пока нет. Создайте проект и загрузите ZIP с сайтом.'),
      ]));
      return;
    }

    const rows = data.projects.slice().sort((a, b) => b.id - a.id).slice(0, 5);
    const tbody = rows.map((p) =>
      h('tr', {}, [
        h('td', {}, [h('span', { class: 'row-link', onclick: () => { location.hash = '#/projects/' + p.id; } }, escapeHtml(p.name))]),
        h('td', { class: 'mono word-break', html: escapeHtml(p.domain) }),
        h('td', {}, [statusBadge(p)]),
        h('td', {}, [p.ssl_enabled ? badge('on') : badge('off')]),
        h('td', { class: 'num', dataset: { projectSize: p.id }, html: formatBytes(p.disk_bytes) }),
      ])
    );
    page.append(h('div', { class: 'card mt-16' }, [
      h('div', { class: 'card-head' }, [
        h('div', {}, [h('div', { class: 'card-title', html: 'Последние проекты' })]),
        h('a', { href: '#/projects', html: 'Все проекты →' }),
      ]),
      h('div', { class: 'table-wrap' }, [
        h('table', { class: 'table' }, [
          h('thead', {}, [h('tr', {}, [
            h('th', {}, 'Название'), h('th', {}, 'Домен'), h('th', {}, 'Статус'), h('th', {}, 'SSL'), h('th', { class: 'num' }, 'Размер'),
          ])]),
          h('tbody', {}, tbody),
        ]),
      ]),
    ]));
  } catch (e) {
    page.append(h('div', { class: 'card empty-state' }, e.message));
  }
}

/* ================= ПРОЕКТЫ ================= */

async function renderProjectsList(page) {
  page.innerHTML = '';
  page.append(h('div', { class: 'page-head' }, [
    h('div', {}, [
      h('div', { class: 'page-title', html: 'Проекты' }),
      h('div', { class: 'page-sub', html: 'Домены, развёрнутые на этом хостинге' }),
    ]),
    h('div', { class: 'page-actions' }, [
      h('button', { class: 'btn btn-primary', onclick: createProjectModal }, ICONS.upload + ' Новый проект'),
    ]),
  ]));

  try {
    const data = await api.get('/api/projects');
    state.view = { data: data.projects, sortKey: 'id', sortDir: -1, filter: '' };
    const usageNotice = h('div', {}, [storageNotice(data.usage_status)]);
    page.append(usageNotice);

    page.append(h('div', { class: 'card' }, [
      h('div', { class: 'filter-bar' }, [
        h('input', { class: 'input', placeholder: 'Поиск по имени или домену…', oninput: (e) => {
          state.view.filter = e.target.value.toLowerCase();
          renderProjectsTable();
        } }),
      ]),
      h('div', { class: 'table-wrap', id: 'projects-table-wrap' }),
    ]));

    const renderProjectsTable = () => {
      const wrap = document.getElementById('projects-table-wrap');
      if (!wrap) return;
      const { data, sortKey, sortDir, filter } = state.view;
      let rows = data.slice();
      if (filter) rows = rows.filter((p) => (p.name + ' ' + p.domain).toLowerCase().includes(filter));
      rows.sort((a, b) => {
        const va = a[sortKey], vb = b[sortKey];
        const r = (typeof va === 'string' ? va.localeCompare(vb) : va - vb);
        return r * sortDir;
      });

      if (rows.length === 0) {
        wrap.innerHTML = '';
        wrap.append(h('div', { class: 'empty-state', html: 'Проекты не найдены. Создайте первый проект.' }));
        return;
      }

      const th = (label, key, cls) => h('th', {
        class: cls || '',
        onclick: () => {
          if (state.view.sortKey === key) state.view.sortDir *= -1;
          else { state.view.sortKey = key; state.view.sortDir = 1; }
          renderProjectsTable();
        },
      }, [label, state.view.sortKey === key ? ' <span class="sort-arrow">' + (state.view.sortDir === 1 ? '▲' : '▼') + '</span>' : null]);

      wrap.innerHTML = '';
      wrap.append(h('table', { class: 'table' }, [
        h('thead', {}, [h('tr', {}, [
          th('Название', 'name'), th('Домен', 'domain'), th('Статус', 'status'), th('SSL', 'ssl_enabled'),
          th('Размер', 'disk_bytes', 'num'), th('Изменён', 'updated_at'), th('Создан', 'created_at'), th('', ''),
        ])]),
        h('tbody', {}, rows.map((p) =>
          h('tr', {}, [
            h('td', {}, [h('span', { class: 'row-link', onclick: () => { location.hash = '#/projects/' + p.id; } }, escapeHtml(p.name))]),
            h('td', { class: 'mono word-break', html: escapeHtml(p.domain) }),
            h('td', {}, [statusBadge(p)]),
            h('td', {}, [p.ssl_enabled ? badge('on') : badge('off')]),
            h('td', { class: 'num', dataset: { projectSize: p.id }, html: formatBytes(p.disk_bytes) }),
            h('td', { class: 'muted', html: formatDate(p.updated_at) }),
            h('td', { class: 'muted', html: formatDate(p.created_at) }),
            h('td', {}, [
              h('button', { class: 'btn btn-sm btn-danger btn-icon', title: 'Удалить', onclick: () => deleteProject(p) }, ICONS.trash),
            ]),
          ])
        )),
      ]));
    };
    renderProjectsTable();
    startPagePolling(page, async () => {
      const current = await api.get('/api/projects', { silent: true });
      if (!page.isConnected) return;
      state.view.data = current.projects;
      usageNotice.replaceChildren(storageNotice(current.usage_status));
      renderProjectsTable();
    });
  } catch (e) {
    page.append(h('div', { class: 'card empty-state' }, e.message));
  }
}

function createProjectModal() {
  const body = h('div', {}, [
    h('div', { class: 'field' }, [
      h('label', { html: 'Имя проекта' }),
      h('input', { class: 'input', id: 'np-name', placeholder: 'my-site', autocapitalize: 'none' }),
      h('div', { class: 'hint', html: '2-64 символа: a-z, 0-9, _-. Регистр не важен.' }),
    ]),
    h('div', { class: 'field' }, [
      h('label', {}, 'Тип проекта'),
      h('select', { class: 'input', id: 'np-provider' }, [
        h('option', { value: 'nginx' }, 'nginx — статический сайт'),
        h('option', { value: 'php-fpm' }, 'PHP-FPM — PHP 8.2+'),
      ]),
      h('div', { class: 'hint' }, 'SSL, метрики и загрузка файлов доступны для обоих типов.'),
    ]),
    h('div', { class: 'field' }, [
      h('label', { html: 'Домен' }),
      h('input', { class: 'input', id: 'np-domain', placeholder: 'example.com', autocapitalize: 'none' }),
      h('div', { class: 'hint', html: 'Домен должен указывать на IP этого сервера. Сайт сразу подключается к nginx.' }),
    ]),
  ]);
  const foot = [
    h('button', { class: 'btn', onclick: closeModal }, 'Отмена'),
    h('button', { class: 'btn btn-primary', onclick: async () => {
      try {
        await api.post('/api/projects', {
          name: document.getElementById('np-name').value.trim(),
          domain: document.getElementById('np-domain').value.trim(),
          provider: document.getElementById('np-provider').value,
        });
        closeModal();
        toast('Проект создан и подключён к nginx');
        render();
      } catch (e) {
        toast(e.message, 'err');
      }
    } }, 'Создать'),
  ];
  openModal('Новый проект', body, foot);
}

async function deleteProject(p) {
  const ok = await confirmDialog('Удаление проекта', 'Удалить проект <b>' + escapeHtml(p.name) + '</b> (' + escapeHtml(p.domain) + ')?<br>Файлы сайта, конфиг nginx, сертификаты и метрики будут удалены с диска безвозвратно.', true);
  if (!ok) return;
  try {
    await api.del('/api/projects/' + p.id);
    toast('Проект удалён');
    if (route()[0] === 'projects' && route()[1]) location.hash = '#/projects';
    else render();
  } catch (e) {
    toast(e.message, 'err');
  }
}

/* ================= ДЕТАЛИ ПРОЕКТА ================= */

function statusBadge(p) {
  if (!p.enabled) return badge('disabled');
  return badge(p.status);
}

async function renderProjectDetail(page, id) {
  stopPagePolling();
  page.innerHTML = '';
  if (!page.isConnected) return;
  page.append(skeleton());

  let p;
  try {
    const data = await api.get('/api/projects/' + id);
    p = data.project;
    if (!page.isConnected) return;
  } catch (e) {
    page.innerHTML = '';
    page.append(h('div', { class: 'card empty-state' }, e.message));
    return;
  }

  page.innerHTML = '';
  page.append(breadcrumbs([
    { href: '#/', label: 'Панель' },
    { href: '#/projects', label: 'Проекты' },
    { label: p.name },
  ]));

  const refreshDetail = () => renderProjectDetail(page, id);

  const uploadUI = createUploadUI();

  const actions = h('div', { class: 'page-actions' }, [
    h('button', { class: 'btn', onclick: refreshDetail }, ICONS.refresh + ' Обновить'),
    h('button', { class: 'btn', disabled: !p.enabled, title: p.enabled ? '' : 'Проект выключен', onclick: () => window.open(p.site_url, '_blank') }, ICONS.external + ' Открыть сайт'),
    h('button', { class: 'btn', onclick: async () => { await api.download('/api/projects/' + p.id + '/download', p.name + '.zip'); toast('Архив подготовлен, скачивание начато'); } }, ICONS.download + ' Скачать'),
    h('label', { class: 'btn btn-primary' }, [
      ICONS.upload, 'Загрузить ZIP',
      h('input', {
        type: 'file', accept: '.zip', style: 'display:none',
        onchange: (e) => {
          const file = e.target.files[0];
          e.target.value = '';
          return uploadZip(p, file, uploadUI);
        },
      }),
    ]),
  ]);

  page.append(h('div', { class: 'page-head' }, [
    h('div', {}, [
      h('div', { class: 'page-title flex' }, [escapeHtml(p.name), statusBadge(p), p.ssl_enabled ? badge('ssl') : null]),
      h('div', { class: 'page-sub mono' }, p.domain),
    ]),
    actions,
  ]));

  page.append(uploadUI.panel);

  const providerSelect = h('select', { class: 'input' }, [
    h('option', { value: '', disabled: true, selected: !p.provider }, 'Существующая настройка сервера'),
    h('option', { value: 'nginx', selected: p.provider === 'nginx' }, 'nginx — статический сайт'),
    h('option', { value: 'php-fpm', selected: p.provider === 'php-fpm' }, 'PHP-FPM — отдельный пул'),
  ]);
  page.append(h('div', { class: 'card mb-24' }, [
    h('div', { class: 'card-head' }, [h('div', { class: 'card-title' }, 'Тип проекта')]),
    h('div', { class: 'card-pad' }, [providerSelect, h('button', { class: 'btn mt-8', onclick: async (e) => {
      if (!providerSelect.value) return;
      e.currentTarget.disabled = true;
      try { await api.put('/api/projects/' + p.id + '/provider', { provider: providerSelect.value }); toast('Тип проекта сохранён'); refreshDetail(); }
      catch (err) { toast(err.message, 'err'); e.currentTarget.disabled = false; }
    } }, 'Сохранить')]),
  ]));
  if (p.provider === 'php-fpm') {
    const directories = h('div', { class: 'card-pad' }, [skeleton()]);
    page.append(h('div', { class: 'card mb-24' }, [
      h('div', { class: 'card-head' }, [
        h('div', { class: 'card-title' }, 'Каталоги данных'),
        h('button', { class: 'btn btn-sm', onclick: () => createDataDirectory(p, refreshDetail) }, 'Создать каталог'),
      ]), directories,
    ]));
    loadDataDirectories(p, directories, refreshDetail);
  }

  const toggle = h('label', { class: 'switch', title: p.enabled ? 'Выключить сайт' : 'Включить сайт' }, [
    h('input', {
      type: 'checkbox',
      checked: !!p.enabled,
      onchange: async (e) => {
        const enabled = e.target.checked;
        e.target.disabled = true;
        try {
          await api.put('/api/projects/' + p.id + '/enabled', { enabled });
          toast(enabled ? 'Сайт включён' : 'Сайт выключен: nginx больше не обслуживает ' + p.domain);
          refreshDetail();
        } catch (err) {
          e.target.checked = !enabled;
          e.target.disabled = false;
          toast(err.message, 'err');
        }
      },
    }),
    h('span', { class: 'slider' }),
  ]);

  page.append(h('div', { class: 'card mb-24' }, [
    h('div', { class: 'card-head' }, [
      h('div', {}, [
        h('div', { class: 'card-title', html: 'Сайт ' + (p.enabled ? 'включён' : 'выключен') }),
        h('div', { class: 'card-sub', html: p.enabled
          ? 'nginx обслуживает домен и его www-вариант'
          : 'Запросы к домену не попадают на сайт. Файлы, настройки и сертификат сохранены.' }),
      ]),
      toggle,
    ]),
    h('div', { class: 'card-pad' }, [
      h('div', { class: 'info-list' }, [
        infoItem('Домен', p.domain),
        infoItem('Адрес сайта', p.site_url),
        infoItem('Статус', !p.enabled ? 'Выключен' : p.status === 'active' ? 'Активен' : 'Ошибка'),
        h('div', { class: 'info-item' }, [h('div', { class: 'info-label' }, 'Хранилище проекта'), h('div', { class: 'info-value', id: 'detail-disk-value' }, formatBytes(p.disk_bytes))]),
        infoItem('Создан', formatDate(p.created_at)),
        infoItem('Последнее изменение', p.updated_at ? formatDate(p.updated_at) : 'файлы ещё не загружались'),
        infoItem('SSL-сертификат', p.ssl_enabled ? 'Установлен' : 'Не установлен'),
        infoItem('SSL выдан', formatDate(p.ssl_issued_at)),
        infoItem('SSL действует до', formatDate(p.ssl_expires_at)),
        p.path ? infoItem('Путь к папке (видит только администратор)', p.path) : null,
      ]),
    ]),
  ]));

  const sslText = p.ssl_enabled
    ? 'Сертификат действует до <b>' + escapeHtml(formatDate(p.ssl_expires_at)) + '</b> и продлевается автоматически. HTTP-запросы перенаправляются на HTTPS.'
    : 'После выпуска сертификата сайт откроется по HTTPS, а запросы на порт 80 будут перенаправляться на 443.';

  page.append(h('div', { class: 'grid grid-2 mb-24' }, [
    h('div', { class: 'card', id: 'metrics-card' }, [h('div', { class: 'card-head' }, [h('div', { class: 'card-title', html: 'Метрики' }), h('button', { class: 'btn btn-sm', onclick: async () => {
      const data = await api.post('/api/projects/' + p.id + '/metrics/refresh');
      if (!page.isConnected) return;
      await loadMetrics(p, false, data); await refreshMe(true); toast('Метрики пересчитаны по файлам и access.log');
    } }, 'Обновить замер')]), h('div', { id: 'metrics-body' }, [skeleton()])]),
    h('div', { class: 'card' }, [
      h('div', { class: 'card-head' }, [
        h('div', {}, [h('div', { class: 'card-title', html: 'SSL-сертификат' })]),
        h('button', { class: 'btn btn-sm btn-primary', disabled: !p.enabled, onclick: (e) => applySSL(p, e.currentTarget) },
          ICONS.shield + (p.ssl_enabled ? ' Перевыпустить' : ' Выпустить SSL')),
      ]),
      h('div', { class: 'card-pad' }, [
        h('div', { html: sslText }),
        !p.enabled ? h('div', { class: 'hint mt-8', html: 'Проект выключен — включите его, чтобы выпустить сертификат.' }) : null,
      ]),
    ]),
  ]));

  page.append(h('div', { class: 'card mb-24' }, [
    h('div', { class: 'card-head' }, [h('div', { class: 'card-title', html: 'Файлы проекта' })]),
    h('div', { id: 'files-body', class: 'card-pad' }, [skeleton()]),
  ]));

  page.append(h('div', { class: 'card' }, [
    h('div', { class: 'card-head' }, [
      h('div', { class: 'card-title', html: 'Опасная зона' }),
    ]),
    h('div', { class: 'card-pad' }, [
      h('button', { class: 'btn btn-danger', onclick: () => deleteProject(p) }, ICONS.trash + ' Удалить проект'),
    ]),
  ]));

  await Promise.all([loadMetrics(p), loadFiles(p)]);
  if (page.isConnected) {
    let polling = false;
    pagePoll = setInterval(async () => {
      if (polling || document.visibilityState !== 'visible' || !page.isConnected || page.querySelector('[aria-busy="true"]')) return;
      polling = true;
      try { await loadMetrics(p, true); } finally { polling = false; }
    }, 10000);
  }
}

function createDataDirectory(p, refresh) {
  const name = h('input', { class: 'input', placeholder: 'app-data', maxlength: 32 });
  const env = h('input', { class: 'input', placeholder: 'APP_DATA_DIR', maxlength: 64 });
  const body = h('div', {}, [
    h('div', { class: 'field' }, [h('label', {}, 'Имя каталога'), name]),
    h('div', { class: 'field' }, [h('label', {}, 'Переменная окружения PHP'), env]),
    h('div', { class: 'hint' }, 'Папка создаётся вне корня сайта с правами 0700. PHP получает её абсолютный путь. Обновление ZIP не затрагивает данные. При удалении проекта данные удаляются.'),
  ]);
  openModal('Каталог данных', body, [
    h('button', { class: 'btn', onclick: closeModal }, 'Отмена'),
    h('button', { class: 'btn btn-primary', onclick: async (e) => {
      const button = e.currentTarget; button.disabled = true;
      try {
        await api.post('/api/projects/' + p.id + '/data-directories', { name: name.value.trim(), env: env.value.trim() });
        closeModal(); toast('Каталог создан, переменная передана в PHP'); refresh();
      } catch (err) { button.disabled = false; toast(err.message, 'err'); }
    } }, 'Создать'),
  ]);
}

async function loadDataDirectories(p, target, refresh) {
  try {
    const data = await api.get('/api/projects/' + p.id + '/data-directories');
    target.replaceChildren(h('div', { class: 'hint mb-24' }, 'Данные хранятся отдельно от файлов сайта и учитываются в дисковой квоте. Конфигурационные файлы можно создать или загрузить здесь; базы SQLite изменяет приложение.'));
    if (!data.directories.length) target.append(h('p', {}, 'Каталоги ещё не созданы.'));
    for (const dir of data.directories) {
      const row = h('div', { class: 'mb-24' }, [
        infoItem('Каталог', dir.name), infoItem(dir.env, dir.path),
        h('button', { class: 'btn btn-sm', onclick: () => editDataFile(p, dir, null, refresh) }, 'Создать файл'),
      ]);
      for (const file of dir.files) row.append(h('button', { class: 'btn btn-sm', onclick: () => editDataFile(p, dir, file, refresh) }, document.createTextNode(file)));
      target.append(row);
    }
  } catch (err) { target.replaceChildren(h('div', {}, err.message)); }
}

async function editDataFile(p, dir, file, refresh) {
  const name = h('input', { class: 'input', value: file || '', placeholder: 'config.json', readonly: !!file });
  const content = h('textarea', { class: 'input mono', rows: 12, placeholder: 'Содержимое файла' });
  const path = (filename) => '/api/projects/' + p.id + '/data-directories/' + encodeURIComponent(dir.name) + '/files/' + encodeURIComponent(filename);
  if (file) {
    try { content.value = (await api.get(path(file))).content; }
    catch (err) { toast(err.message, 'err'); return; }
  }
  const upload = h('input', { type: 'file', onchange: async (e) => {
    const selected = e.target.files[0]; if (!selected) return;
    if (selected.size > 1024 * 1024) { toast('Максимальный размер файла — 1 МБ', 'err'); return; }
    if (!file) name.value = selected.name;
    content.value = await selected.text();
  } });
  openModal('Файл данных', h('div', {}, [
    h('div', { class: 'field' }, [h('label', {}, 'Имя файла'), name]),
    h('div', { class: 'field' }, [h('label', {}, 'Загрузить текстовый файл'), upload]),
    h('div', { class: 'field' }, [h('label', {}, 'Содержимое'), content]),
    h('div', { class: 'hint' }, 'Редактор предназначен для конфигурации. Не редактируйте работающую базу данных.'),
  ]), [
    h('button', { class: 'btn', onclick: closeModal }, 'Отмена'),
    h('button', { class: 'btn btn-primary', onclick: async (e) => {
      const button = e.currentTarget; button.disabled = true;
      try { await api.put(path(name.value.trim()), { content: content.value }); closeModal(); toast('Файл сохранён'); refresh(); }
      catch (err) { button.disabled = false; toast(err.message, 'err'); }
    } }, 'Сохранить'),
  ]);
}

function infoItem(label, value) {
  return h('div', { class: 'info-item' }, [
    h('div', { class: 'info-label', html: label }),
    h('div', { class: 'info-value' }, value == null || value === '' ? '—' : String(value)),
  ]);
}

function createUploadUI() {
  let fileName = '';
  let busy = false;
  const label = h('div', { class: 'upload-label' }, '');
  const percent = h('div', { class: 'upload-pct' }, '');
  const bar = h('div', { class: 'progress-bar', style: 'width:0%' });
  const panel = h('div', { class: 'card upload-card mb-24', style: 'display:none' }, [
    h('div', { class: 'card-pad' }, [
      h('div', { class: 'flex-between' }, [label, percent]),
      h('div', { class: 'progress mt-8' }, [bar]),
      h('div', { class: 'upload-hint muted' }, 'Не закрывайте вкладку, пока загрузка и распаковка не завершатся.'),
    ]),
  ]);

  const show = (text, pct, cls) => {
    panel.style.display = '';
    label.textContent = text;
    percent.textContent = pct + '%';
    bar.style.width = pct + '%';
    bar.className = 'progress-bar' + (cls ? ' ' + cls : '');
  };

  return {
    panel,
    get busy() { return busy; },
    start(file) {
      busy = true;
      fileName = file.name || 'архив';
      show('Загрузка ' + fileName + '…', 0);
    },
    progress(frac) {
      const pct = Math.max(0, Math.min(100, Math.round(frac * 100)));
      if (pct >= 100) show('Файл передан. Сервер распаковывает архив…', 100);
      else show('Загрузка ' + fileName + '…', pct);
    },
    fail(message) {
      busy = false;
      show('Ошибка загрузки: ' + message, 0, 'danger');
    },
  };
}

async function uploadZip(p, file, ui) {
  if (!file) return;
  if (ui && ui.busy) {
    toast('Дождитесь завершения текущей загрузки', 'warn');
    return;
  }
  if (ui) ui.start(file);
  const fd = new FormData();
  fd.append('file', file);
  try {
    const res = await api.upload('/api/projects/' + p.id + '/upload', fd, (frac) => {
      if (ui) ui.progress(frac);
    });
    toast('Архив загружен: ' + formatBytes(res.written) + ', файлы распакованы');
    render();
  } catch (e) {
    if (ui) ui.fail(e.message);
    toast(e.message, 'err');
  }
}

async function applySSL(p, btn) {
  if (btn) btn.disabled = true;
  try {
    const res = await api.post('/api/projects/' + p.id + '/apply-ssl');
    const np = res.project;
    const lines = [
      'Сертификат выпущен: действует с ' + escapeHtml(formatDate(np.ssl_issued_at)) + ' до <b>' + escapeHtml(formatDate(np.ssl_expires_at)) + '</b>.',
      'Сайт доступен по <span class="mono">' + escapeHtml(np.site_url) + '</span>, HTTP перенаправляется на HTTPS.',
    ];
    if (res.provider === 'selfsigned') {
      lines.push('Домен локальный, поэтому сертификат подписан локальным CA панели (Let’s Encrypt такие домены не проверяет). Браузер покажет предупреждение, пока CA не добавлен в доверенные'
        + (res.ca_file ? ': <span class="mono">' + escapeHtml(res.ca_file) + '</span>.' : '.'));
    }
    openModal('SSL-сертификат', h('div', {}, lines.map((l) => h('p', { class: 'mb-16', html: l }))),
      [h('button', { class: 'btn btn-primary', onclick: closeModal }, 'Закрыть')]);
    render();
  } catch (e) {
    openModal('Не удалось выпустить сертификат', h('div', { class: 'code-block' }, e.message),
      [h('button', { class: 'btn btn-primary', onclick: closeModal }, 'Закрыть')], { size: 'wide' });
    if (btn) btn.disabled = false;
  }
}

function metricRates(series) {
  return series.slice(1).map((point, i) => {
    const before = series[i];
    const seconds = (point.timestamp_ms - before.timestamp_ms) / 1000;
    const requests = point.requests_cum - before.requests_cum;
    return seconds > 0 && requests >= 0 ? requests * 60 / seconds : null;
  });
}

function localTimestamp(value) {
  return value ? new Date(value).toLocaleString('ru-RU') : 'ещё не было';
}

async function loadMetrics(p, silent = false, supplied = null) {
  const body = document.getElementById('metrics-body');
  if (!body) return;
  try {
    const data = supplied || await api.get('/api/projects/' + p.id + '/metrics', { silent });
    if (!body.isConnected) return;
    const series = data.series || [];
    const labels = series.map((m) => new Date(m.timestamp_ms).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }));
    const rates = metricRates(series);
    const statusText = {
      ok: 'Сборщик работает', pending: 'Ожидается первый замер',
      stale: 'Данные устарели: проверьте сборщик', error: 'Последний сбор завершился ошибкой',
    }[data.status] || 'Состояние неизвестно';
    const button = document.querySelector('#metrics-card button');
    if (button && !button.hasAttribute('aria-busy')) {
      button.disabled = !data.can_refresh;
      button.title = data.can_refresh ? 'Прочитать файлы и access.log сейчас' : 'Встроенный сборщик выключен; используется отдельный сервис';
    }
    const storage = data.storage;
    const c1 = h('canvas', { 'aria-label': 'Размер файлов проекта во времени', role: 'img' });
    const c2 = h('canvas', { 'aria-label': 'Запросы в минуту', role: 'img' });
    const latestRate = rates.length ? rates[rates.length - 1] : null;
    body.replaceChildren(h('div', { class: 'card-pad' }, [
      h('div', { class: 'metric-status ' + data.status, role: 'status' }, [
        h('strong', {}, statusText + (data.running ? ' · идёт замер…' : '')),
        h('div', {}, 'Замер: ' + localTimestamp(data.updated_at)),
        h('div', { class: 'hint' }, 'Сбор каждые ' + data.interval_seconds + ' с · экран обновляется каждые 10 с'),
        data.error ? h('div', {}, document.createTextNode(data.error)) : null,
        !data.can_refresh ? h('div', { class: 'hint' }, 'Отдельный сборщик: ручной замер из панели недоступен') : null,
      ]),
      h('div', { class: 'metrics-summary' }, [
        h('div', {}, [h('div', { class: 'muted' }, 'Хранилище проекта'), h('div', { class: 'metric-value', id: 'project-disk-value' }, formatBytes(data.disk_bytes))]),
        h('div', {}, [h('div', { class: 'muted' }, 'Запросов / мин'), h('div', { class: 'metric-value' }, latestRate == null ? 'Нет интервала' : latestRate.toLocaleString('ru-RU', { maximumFractionDigits: 2 }))]),
        h('div', {}, [h('div', { class: 'muted' }, 'Учтённый трафик'), h('div', { class: 'metric-value' }, formatBytes(data.bandwidth_cum))]),
      ]),
      storage ? h('div', { class: 'hint mb-24' }, 'Файлы сайта: ' + formatBytes(storage.site_bytes) + ' · Данные PHP: ' + formatBytes(storage.data_bytes) + ' · Логи: ' + formatBytes(storage.log_bytes) + ' (вне квоты)') : null,
      h('div', { class: 'hint mb-24' }, 'Хранилище — размер файлов на диске. Это не оперативная память (RAM).'),
      series.length ? h('div', {}, [
        h('div', { class: 'chart-legend' }, 'Размер файлов сайта и данных PHP'),
        h('div', { class: 'chart-box' }, [c1]),
        rates.some((v) => v != null) ? h('div', {}, [h('div', { class: 'chart-legend mt-16' }, 'Запросы / мин · с учётом фактической длительности интервала'), h('div', { class: 'chart-box' }, [c2])])
          : h('div', { class: 'hint mt-16' }, 'Для скорости запросов нужны два замера с разным временем.'),
      ]) : h('div', { class: 'empty-state' }, 'Истории пока нет. Нажмите «Обновить замер» или дождитесь фонового сбора.'),
      h('div', { class: 'mt-16 muted' }, 'Всего учтено запросов: ' + formatNumber(data.requests_cum) + ' · История: ' + series.length + ' замеров'),
    ]));
    p.disk_bytes = data.disk_bytes;
    const detailDisk = document.getElementById('detail-disk-value');
    if (detailDisk) detailDisk.textContent = formatBytes(data.disk_bytes);
    requestAnimationFrame(() => {
      if (!body.isConnected) return;
      if (series.length) chart(c1, labels, [series.map((m) => m.disk_bytes)], { format: formatBytes, padL: 68, height: 140 });
      if (rates.some((v) => v != null)) chart(c2, labels.slice(1), [rates], { format: (v) => v.toLocaleString('ru-RU', { maximumFractionDigits: 1 }), height: 140, colors: ['#12B76A'] });
    });
  } catch (e) {
    if (!body.isConnected) return;
    const previous = body.querySelector('.metrics-summary');
    if (previous) {
      const status = body.querySelector('.metric-status');
      status.className = 'metric-status error'; status.textContent = 'Не удалось обновить. Показаны предыдущие данные: ' + e.message;
    } else body.replaceChildren(h('div', { class: 'card-pad metric-status error' }, 'Метрики недоступны: ' + e.message));
  }
}

async function loadFiles(p) {
  const body = document.getElementById('files-body');
  try {
    const data = await api.get('/api/projects/' + p.id + '/files');
    body.innerHTML = '';
    if (!data.files || data.files.length === 0) {
      body.append(h('div', { class: 'empty-state', html: 'Файлы не загружены. Загрузите ZIP-архив с index.html или index.php.' }));
      return;
    }
    const list = h('ul', { class: 'file-list' });
    for (const f of data.files) {
      list.append(h('li', {}, [
        h('span', { html: f.dir ? ICONS.folder : ICONS.file }),
        h('span', { class: 'f-path', html: escapeHtml(f.path) }),
        h('span', { class: 'f-size', html: f.dir ? '—' : formatBytes(f.size) }),
      ]));
    }
    body.append(list);
  } catch (e) {
    body.innerHTML = '';
    body.append(h('div', { class: 'empty-state' }, e.message));
  }
}

/* ================= НАСТРОЙКИ ================= */

function githubActionsWorkflow(projectId) {
  const id = /^[1-9]\d*$/.test(String(projectId)) ? String(projectId) : '<ID_ПРОЕКТА>';
  return [
    'name: Deploy to FastHost',
    '',
    'on:',
    '  push:',
    '    branches: [main]',
    '  workflow_dispatch:',
    '',
    'permissions:',
    '  contents: read',
    '',
    'concurrency:',
    '  group: fasthost-${{ github.repository }}-' + id,
    '  cancel-in-progress: false',
    '',
    'jobs:',
    '  deploy:',
    '    runs-on: ubuntu-latest',
    '    timeout-minutes: 10',
    '    env:',
    "      FASTHOST_PROJECT_ID: '" + id + "'",
    '    steps:',
    '      - uses: actions/checkout@v6',
    '        with:',
    '          persist-credentials: false',
    '',
    '      - name: Set up PHP',
    '        uses: shivammathur/setup-php@v2',
    '        with:',
    "          php-version: '8.4'",
    '          extensions: pdo_sqlite',
    '          coverage: none',
    '',
    '      - name: Lint PHP',
    '        run: |',
    "          find . -path './vendor' -prune -o -path './.git' -prune -o \\",
    "            -type f -name '*.php' -print0 | xargs -0 -r -n1 php -l",
    '',
    '      - name: Package site',
    '        run: |',
    '          zip -r "$RUNNER_TEMP/site.zip" . \\',
    '            -x ".git/*" ".github/*" ".env" ".env.*" \\',
    '               "*.sqlite" "*.sqlite-*" "*.db" "*.db-*" "site.zip"',
    '',
    '      - name: Deploy site',
    '        env:',
    '          FASTHOST_API_KEY: ${{ secrets.FASTHOST_API_KEY }}',
    '        run: |',
    '          test -n "$FASTHOST_API_KEY" || { echo "FASTHOST_API_KEY is missing"; exit 1; }',
    '          curl --fail-with-body --show-error --silent \\',
    '            --connect-timeout 15 --max-time 300 \\',
    '            -H "X-Api-Key: $FASTHOST_API_KEY" \\',
    '            -H "Content-Type: application/zip" \\',
    '            --data-binary @"$RUNNER_TEMP/site.zip" \\',
    '            "https://api.sudzekai.ru/fast-host/api/deploy/$FASTHOST_PROJECT_ID"',
    '',
  ].join('\n');
}

function showAPIKey(key) {
  const el = document.getElementById('api-key-view');
  if (!el) return;
  el.dataset.key = key;
  el.textContent = key;
  el.setAttribute('aria-busy', 'false');
  const copy = document.getElementById('copy-api-key');
  if (copy) copy.disabled = false;
}

async function renderSettings(page) {
  page.replaceChildren(breadcrumbs([{ href: '#/', label: 'Панель' }, { label: 'Настройки' }]));
  page.append(h('div', { class: 'page-head' }, [h('div', {}, [
    h('div', { class: 'page-title' }, 'Настройки'),
    h('div', { class: 'page-sub' }, 'Доступ к аккаунту и автоматический деплой проектов'),
  ])]));
  const cardHeading = (icon, title, subtitle) => h('div', { class: 'card-head' }, [
    h('div', { class: 'settings-heading' }, [h('span', { class: 'settings-icon', 'aria-hidden': 'true', html: icon }),
      h('div', {}, [h('div', { class: 'card-title' }, title), h('div', { class: 'card-sub' }, subtitle)]),
    ]),
  ]);
  const copyKey = h('button', { class: 'btn btn-primary', id: 'copy-api-key', disabled: true, onclick: async () => {
    const key = document.getElementById('api-key-view').dataset.key;
    if (!key) return;
    await navigator.clipboard.writeText(key); toast('API-ключ скопирован');
  } }, ICONS.copy + ' Копировать ключ');
  const keyCard = h('section', { class: 'card' }, [
    cardHeading(ICONS.upload, 'API-ключ деплоя', 'Один ключ для ваших проектов и GitHub Actions'),
    h('div', { class: 'card-pad' }, [
      h('div', { class: 'field' }, [h('label', {}, 'Ваш ключ'),
        h('div', { class: 'settings-key mono', id: 'api-key-view', role: 'status', 'aria-busy': 'true' }, [h('span', { class: 'spinner spinner-inline' }), ' Загружаем ключ…']),
      ]),
      h('div', { class: 'settings-actions' }, [copyKey,
        h('button', { class: 'btn', onclick: async () => {
          if (!await confirmDialog('Перевыпустить ключ', 'Старый ключ перестанет работать во всех ваших проектах. После перевыпуска обновите секрет FASTHOST_API_KEY в GitHub.', true)) return;
          const result = await api.post('/api/users/me/api-key/regenerate');
          if (!page.isConnected) return;
          showAPIKey(result.api_key); toast('Ключ перевыпущен');
        } }, ICONS.refresh + ' Перевыпустить'),
      ]),
      h('p', { class: 'hint mt-16' }, 'Добавьте ключ в секреты репозитория. Пример ниже ссылается на секрет и не содержит ваш ключ.'),
    ]),
  ]);
  const current = h('input', { class: 'input', type: 'password', id: 'pw-cur', autocomplete: 'current-password', required: true });
  const password = h('input', { class: 'input', type: 'password', id: 'pw-new', autocomplete: 'new-password', minlength: 6, required: true });
  const confirm = h('input', { class: 'input', type: 'password', id: 'pw-confirm', autocomplete: 'new-password', minlength: 6, required: true });
  const passwordCard = h('section', { class: 'card' }, [
    cardHeading(ICONS.shield, 'Безопасность аккаунта', 'Смена пароля для входа в панель'),
    h('div', { class: 'card-pad' }, [
      h('div', { class: 'field' }, [h('label', { for: 'pw-cur' }, 'Текущий пароль'), current]),
      h('div', { class: 'field' }, [h('label', { for: 'pw-new' }, 'Новый пароль'), password, h('div', { class: 'hint' }, 'Не менее 6 символов')]),
      h('div', { class: 'field' }, [h('label', { for: 'pw-confirm' }, 'Подтверждение пароля'), confirm]),
      h('button', { class: 'btn btn-primary', onclick: async () => {
        if (!current.value) { current.focus(); toast('Укажите текущий пароль', 'err'); return; }
        if (password.value.length < 6) { password.focus(); toast('Новый пароль — не короче 6 символов', 'err'); return; }
        if (password.value !== confirm.value) { confirm.focus(); toast('Пароли не совпадают', 'err'); return; }
        await api.put('/api/users/me/password', { current_password: current.value, new_password: password.value });
        current.value = password.value = confirm.value = ''; toast('Пароль изменён');
      } }, 'Сменить пароль'),
    ]),
  ]);
  page.append(h('div', { class: 'grid grid-2 settings-grid mb-24' }, [keyCard, passwordCard]));

  const project = h('select', { class: 'input', id: 'workflow-project', disabled: true, 'aria-label': 'Проект для GitHub Actions' }, [h('option', { value: '' }, 'Загружаем проекты…')]);
  const code = h('code', { id: 'workflow-code' }, document.createTextNode(githubActionsWorkflow('')));
  const endpoint = h('div', { class: 'settings-endpoint mono' }, 'POST /api/deploy/<ID_ПРОЕКТА>');
  project.addEventListener('change', () => {
    code.textContent = githubActionsWorkflow(project.value);
    endpoint.textContent = 'POST /api/deploy/' + (project.value || '<ID_ПРОЕКТА>');
  });
  const projectsStatus = h('div', { class: 'hint mt-8', role: 'status' }, 'ID проекта появится в примере после выбора.');
  const copyWorkflow = h('button', { class: 'btn btn-primary btn-sm', onclick: async () => {
    await navigator.clipboard.writeText(code.textContent); toast('Workflow скопирован');
  } }, ICONS.copy + ' Копировать YAML');
  page.append(h('section', { class: 'card' }, [
    cardHeading(ICONS.projects, 'Автоматический деплой', 'Готовый пример GitHub Actions для PHP-сайта'),
    h('div', { class: 'card-pad settings-deploy-grid' }, [
      h('div', { class: 'settings-instructions' }, [
        h('div', { class: 'field' }, [h('label', { for: 'workflow-project' }, 'Проект для деплоя'), project, projectsStatus]), endpoint,
        h('ol', { class: 'settings-steps' }, [
          h('li', {}, [h('strong', {}, 'Добавьте секрет'), h('p', {}, 'GitHub → Settings → Secrets and variables → Actions → New repository secret. Имя: FASTHOST_API_KEY. Значение: ваш ключ из карточки выше.')]),
          h('li', {}, [h('strong', {}, 'Сохраните workflow'), h('p', {}, 'Создайте в репозитории файл .github/workflows/deploy.yml и вставьте пример. Выберите проект или замените <ID_ПРОЕКТА> его числовым ID.')]),
          h('li', {}, [h('strong', {}, 'Отправьте изменения в main'), h('p', {}, 'GitHub проверит PHP-файлы, соберёт ZIP и отправит его в FastHost. Результат появится во вкладке Actions. Доступен и ручной запуск.')]),
        ]),
        h('div', { class: 'settings-note' }, [h('strong', {}, 'Состав архива'), h('p', {}, 'В корне репозитория должен быть index.php, index.html или index.htm. Если проект использует Composer, добавьте composer install перед упаковкой. Пример не запускает сборку фронтенда.')]),
        h('div', { class: 'settings-note' }, [h('strong', {}, 'Файлы сайта заменяются целиком'), h('p', {}, 'При деплое старые файлы, которых нет в ZIP, удаляются. Отдельные каталоги данных PHP сохраняются. Файлы .env и базы SQLite в пример архива не входят.')]),
        h('a', { href: 'https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets', target: '_blank', rel: 'noopener noreferrer' }, 'Справка GitHub о секретах ↗'),
      ]),
      h('div', { class: 'settings-workflow' }, [
        h('div', { class: 'code-head settings-code-head' }, [h('span', { class: 'mono' }, '.github/workflows/deploy.yml'), copyWorkflow]),
        h('pre', { class: 'code-block attached settings-code' }, [code]),
      ]),
    ]),
  ]));

  await Promise.allSettled([
    (async () => {
      try { const result = await api.get('/api/users/me/api-key'); if (page.isConnected) showAPIKey(result.api_key); }
      catch (error) { if (page.isConnected) { const view = document.getElementById('api-key-view'); view.removeAttribute('aria-busy'); view.textContent = 'Ключ недоступен: ' + error.message; } }
    })(),
    (async () => {
      try {
        const result = await api.get('/api/projects'); if (!page.isConnected) return;
        project.replaceChildren(h('option', { value: '' }, 'Выберите проект'));
        for (const p of result.projects || []) project.append(h('option', { value: String(p.id) }, document.createTextNode(p.name + ' · ' + p.domain + ' · #' + p.id)));
        project.disabled = !(result.projects || []).length;
        if (project.disabled) projectsStatus.textContent = 'Проектов пока нет. Создайте проект в панели и укажите его ID в примере.';
      } catch (error) {
        if (page.isConnected) { project.replaceChildren(h('option', { value: '' }, 'Список недоступен')); projectsStatus.textContent = 'Не удалось загрузить проекты. ID можно указать в примере вручную.'; }
      }
    })(),
  ]);
}

/* ================= АДМИН: ПОЛЬЗОВАТЕЛИ ================= */

async function renderAdminUsers(page) {
  page.innerHTML = '';
  page.append(breadcrumbs([{ href: '#/', label: 'Панель' }, { label: 'Пользователи' }]));
  page.append(h('div', { class: 'page-head' }, [
    h('div', {}, [
      h('div', { class: 'page-title', html: 'Пользователи' }),
      h('div', { class: 'page-sub', html: 'Учётные записи клиентов, квоты и проекты' }),
    ]),
  ]));

  try {
    const [data, stats] = await Promise.all([api.get('/api/admin/users'), api.get('/api/admin/stats')]);
    const users = data.users || [];

    page.append(h('div', { class: 'card card-pad mb-16 flex-between' }, [
      h('div', {}, [
        h('div', { class: 'card-title', html: 'Регистрация новых клиентов' }),
        h('div', { class: 'card-sub', html: stats.registration_flag_off
          ? 'Жёстко отключена флагом запуска (--allow-registration=false), переключатель недоступен.'
          : 'Разрешить самостоятельную регистрацию со страницы «Регистрация»' }),
      ]),
      stats.registration_flag_off
        ? h('span', { class: 'badge badge-neutral', html: 'Выключена флагом' })
        : h('label', { class: 'switch' }, [
            h('input', {
              type: 'checkbox',
              checked: !!stats.registration_enabled,
              onchange: async (e) => {
                try {
                  await api.put('/api/admin/registration', { enabled: e.target.checked });
                  toast(e.target.checked ? 'Регистрация включена' : 'Регистрация отключена');
                } catch (err) {
                  e.target.checked = !e.target.checked;
                  toast(err.message, 'err');
                }
              },
            }),
            h('span', { class: 'slider' }),
          ]),
    ]));

    state.view = { data: users, sortKey: 'id', sortDir: 1, filter: '' };

    page.append(h('div', { class: 'card' }, [
      h('div', { class: 'filter-bar' }, [
        h('input', { class: 'input', placeholder: 'Поиск по логину…', oninput: (e) => {
          state.view.filter = e.target.value.toLowerCase();
          renderUsersTable();
        } }),
      ]),
      h('div', { class: 'table-wrap', id: 'users-table-wrap' }),
    ]));

    const renderUsersTable = () => {
      const wrap = document.getElementById('users-table-wrap');
      if (!wrap) return;
      const { data, sortKey, sortDir, filter } = state.view;
      let rows = data.slice();
      if (filter) rows = rows.filter((u) => u.username.toLowerCase().includes(filter));
      rows.sort((a, b) => {
        const va = a[sortKey], vb = b[sortKey];
        return (typeof va === 'string' ? va.localeCompare(vb) : va - vb) * sortDir;
      });

      if (rows.length === 0) {
        wrap.innerHTML = '';
        wrap.append(h('div', { class: 'empty-state', html: 'Пользователи не найдены' }));
        return;
      }

      const th = (label, key, cls) => h('th', {
        class: cls || '',
        onclick: () => {
          if (state.view.sortKey === key) state.view.sortDir *= -1;
          else { state.view.sortKey = key; state.view.sortDir = 1; }
          renderUsersTable();
        },
      }, [label, state.view.sortKey === key ? ' <span class="sort-arrow">' + (state.view.sortDir === 1 ? '▲' : '▼') + '</span>' : null]);

      wrap.innerHTML = '';
      wrap.append(h('table', { class: 'table' }, [
        h('thead', {}, [h('tr', {}, [
          th('Логин', 'username'), th('Роль', 'role'), th('Квота', 'quota_bytes', 'num'),
          th('Занято', 'used_bytes', 'num'), th('Проекты', 'project_count', 'num'), th('Создан', 'created_at'), th('', ''),
        ])]),
        h('tbody', {}, rows.map((u) =>
          h('tr', {}, [
            h('td', {}, [h('span', { class: 'row-link', onclick: () => { location.hash = '#/admin/users/' + u.id; } }, escapeHtml(u.username))]),
            h('td', {}, [badge(u.role)]),
            h('td', { class: 'num', html: formatBytes(u.quota_bytes) }),
            h('td', { class: 'num', html: formatBytes(u.used_bytes) }),
            h('td', { class: 'num', html: String(u.project_count) }),
            h('td', { class: 'muted', html: formatDate(u.created_at) }),
            h('td', {}, [
              h('button', { class: 'btn btn-sm', onclick: () => setQuotaModal(u) }, 'Квота'),
              h('button', { class: 'btn btn-sm', onclick: () => setPasswordModal(u) }, 'Пароль'),
            ]),
          ])
        )),
      ]));
    };
    renderUsersTable();
  } catch (e) {
    page.append(h('div', { class: 'card empty-state' }, e.message));
  }
}

function setQuotaModal(u) {
  const body = h('div', {}, [
    h('div', { class: 'field' }, [
      h('label', { html: 'Лимит хранения для ' + escapeHtml(u.username) }),
      h('select', { class: 'input', id: 'q-select' }, [
        ['2', '2 ГБ'], ['3', '3 ГБ'], ['4', '4 ГБ'], ['5', '5 ГБ (по умолчанию)'],
        ['10', '10 ГБ'], ['20', '20 ГБ'], ['50', '50 ГБ'], ['100', '100 ГБ'],
      ].map(([v, l]) => h('option', { value: v, selected: String(u.quota_bytes / 1073741824) === v }, l))),
      h('div', { class: 'hint', html: 'Суммарно все проекты пользователя не более этого лимита. Сейчас занято: ' + formatBytes(u.used_bytes) }),
    ]),
  ]);
  const foot = [
    h('button', { class: 'btn', onclick: closeModal }, 'Отмена'),
    h('button', { class: 'btn btn-primary', onclick: async () => {
      try {
        await api.put('/api/admin/users/' + u.id + '/quota', { quota_bytes: parseInt(document.getElementById('q-select').value, 10) * 1073741824 });
        closeModal();
        toast('Квота обновлена');
        render();
      } catch (e) {
        toast(e.message, 'err');
      }
    } }, 'Сохранить'),
  ];
  openModal('Квота хранения', body, foot);
}

function setPasswordModal(u) {
  const body = h('div', { class: 'field' }, [
    h('label', { html: 'Новый пароль для ' + escapeHtml(u.username) }),
    h('input', { class: 'input', type: 'password', id: 'ap-pass', autocomplete: 'new-password' }),
    h('div', { class: 'hint', html: 'Не короче 6 символов' }),
  ]);
  const foot = [
    h('button', { class: 'btn', onclick: closeModal }, 'Отмена'),
    h('button', { class: 'btn btn-primary', onclick: async () => {
      const pw = document.getElementById('ap-pass').value;
      if (pw.length < 6) { toast('Пароль не короче 6 символов', 'err'); return; }
      try {
        await api.put('/api/admin/users/' + u.id + '/password', { new_password: pw });
        closeModal();
        toast('Пароль изменён');
      } catch (e) {
        toast(e.message, 'err');
      }
    } }, 'Сменить'),
  ];
  openModal('Смена пароля', body, foot);
}

/* ================= АДМИН: ПОЛЬЗОВАТЕЛЬ ================= */

async function renderAdminUserDetail(page, id) {
  page.innerHTML = '';
  page.append(breadcrumbs([
    { href: '#/', label: 'Панель' },
    { href: '#/admin/users', label: 'Пользователи' },
    { label: '#' + id },
  ]));
  page.append(skeleton());

  let data;
  try {
    data = await api.get('/api/admin/users/' + id);
  } catch (e) {
    page.innerHTML = '';
    page.append(h('div', { class: 'card empty-state' }, e.message));
    return;
  }
  page.innerHTML = '';
  const u = data.user;

  page.append(h('div', { class: 'page-head' }, [
    h('div', {}, [
      h('div', { class: 'page-title flex' }, [escapeHtml(u.username), badge(u.role)]),
      h('div', { class: 'page-sub', html: 'Учётная запись клиента хостинга' }),
    ]),
  ]));

  page.append(h('div', { class: 'card card-pad mb-24' }, [
    h('div', { class: 'info-list' }, [
      infoItem('Логин', u.username),
      infoItem('Роль', u.role === 'admin' ? 'Администратор' : 'Пользователь'),
      infoItem('Квота хранения', formatBytes(u.quota_bytes)),
      infoItem('Занято', formatBytes(u.used_bytes)),
      infoItem('Проектов', String(u.project_count)),
      infoItem('Создан', formatDate(u.created_at)),
      infoItem('Каталог пользователя', data.data_dir + '/users/' + u.username),
      infoItem('Путь к проектам', data.data_dir + '/users/' + u.username + '/projects'),
    ]),
  ]));

  page.append(h('div', { class: 'grid grid-2 mb-24' }, [
    h('div', { class: 'card' }, [
      h('div', { class: 'card-head' }, [h('div', { class: 'card-title', html: 'Смена пароля' })]),
      h('div', { class: 'card-pad' }, [
        h('div', { class: 'field' }, [
          h('input', { class: 'input', type: 'password', id: 'ad-pass', placeholder: 'Новый пароль', autocomplete: 'new-password' }),
        ]),
        h('button', { class: 'btn btn-primary', onclick: async () => {
          const pw = document.getElementById('ad-pass').value;
          if (pw.length < 6) { toast('Пароль не короче 6 символов', 'err'); return; }
          try {
            await api.put('/api/admin/users/' + u.id + '/password', { new_password: pw });
            toast('Пароль изменён');
            document.getElementById('ad-pass').value = '';
          } catch (e) { toast(e.message, 'err'); }
        } }, 'Сменить пароль'),
      ]),
    ]),
    h('div', { class: 'card' }, [
      h('div', { class: 'card-head' }, [h('div', { class: 'card-title', html: 'Лимит хранения' })]),
      h('div', { class: 'card-pad' }, [
        h('div', { class: 'field' }, [
          h('select', { class: 'input', id: 'ad-quota' }, [
            ['2', '2 ГБ'], ['3', '3 ГБ'], ['4', '4 ГБ'], ['5', '5 ГБ'], ['10', '10 ГБ'], ['20', '20 ГБ'], ['50', '50 ГБ'], ['100', '100 ГБ'],
          ].map(([v, l]) => h('option', { value: v, selected: String(u.quota_bytes / 1073741824) === v }, l))),
        ]),
        h('div', { class: 'mt-8' }, [progressBar(u.used_bytes, u.quota_bytes)]),
        h('button', { class: 'btn btn-primary mt-16', onclick: async () => {
          try {
            await api.put('/api/admin/users/' + u.id + '/quota', { quota_bytes: parseInt(document.getElementById('ad-quota').value, 10) * 1073741824 });
            toast('Квота обновлена');
            render();
          } catch (e) { toast(e.message, 'err'); }
        } }, 'Сохранить квоту'),
      ]),
    ]),
  ]));

  page.append(h('div', { class: 'card' }, [
    h('div', { class: 'card-head' }, [h('div', { class: 'card-title', html: 'Проекты пользователя' })]),
    h('div', { class: 'table-wrap' }, [
      h('table', { class: 'table' }, [
        h('thead', {}, [h('tr', {}, [
          h('th', {}, 'Название'), h('th', {}, 'Домен'), h('th', {}, 'Статус'), h('th', {}, 'SSL'),
          h('th', { class: 'num' }, 'Размер'), h('th', {}, 'Изменён'), h('th', {}, 'Путь'),
        ])]),
        h('tbody', {}, (data.projects || []).map((p) =>
          h('tr', {}, [
            h('td', {}, [h('span', { class: 'row-link', onclick: () => { location.hash = '#/projects/' + p.id; } }, escapeHtml(p.name))]),
            h('td', { class: 'mono word-break', html: escapeHtml(p.domain) }),
            h('td', {}, [statusBadge(p)]),
            h('td', {}, [p.ssl_enabled ? badge('on') : badge('off')]),
            h('td', { class: 'num', dataset: { projectSize: p.id }, html: formatBytes(p.disk_bytes) }),
            h('td', { class: 'muted', html: formatDate(p.updated_at) }),
            h('td', { class: 'mono muted', html: escapeHtml(p.path || '') }),
          ])
        )),
      ]),
    ]),
  ]));
}

boot();

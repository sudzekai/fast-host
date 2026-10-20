const state = {
  me: null,
  used: 0,
  projectCount: 0,
  view: null,
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

async function refreshMe() {
  try {
    const data = await api.get('/api/me');
    state.me = data.user;
    state.used = data.used_bytes || 0;
    state.projectCount = data.project_count || 0;
    return true;
  } catch (e) {
    state.me = null;
    return false;
  }
}

let renderSeq = 0;

// Перед каждой отрисовкой панели перечитываем /api/me: квота, роль и занятое место
// всегда актуальны, а истёкшая сессия сразу ведёт на страницу входа.
async function render(opts = {}) {
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
    h('button', { class: 'btn btn-primary btn-block', id: 'login-btn' }, 'Войти'),
    h('div', { class: 'auth-switch' }, ['Нет аккаунта? ', h('a', { href: '#/register', html: 'Регистрация' })]),
  ]);
  app.append(h('div', { class: 'auth-wrap' }, [card]));

  const doLogin = async () => {
    const btn = document.getElementById('login-btn');
    btn.disabled = true;
    try {
      await api.post('/api/auth/login', {
        username: document.getElementById('login-user').value.trim(),
        password: document.getElementById('login-pass').value,
      });
      await refreshMe();
      location.hash = '#/';
      render({ skipRefresh: true });
    } catch (e) {
      toast(e.message, 'err');
      btn.disabled = false;
    }
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
    btn.disabled = true;
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
      btn.disabled = false;
    }
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
        h('div', { class: 'stat-value', html: formatBytes(data.used_bytes) }),
        h('div', { class: 'mt-8' }, [progressBar(data.used_bytes, quota)]),
      ]),
      h('div', { class: 'card stat-card' }, [
        h('div', { class: 'stat-label', html: 'Квота хранения' }),
        h('div', { class: 'stat-value', html: formatBytes(quota) }),
        h('div', { class: 'stat-hint', html: 'лимит: ' + pct + '% использовано' }),
      ]),
    ]));

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
        h('td', { class: 'num', html: formatBytes(p.disk_bytes) }),
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
            h('td', { class: 'num', html: formatBytes(p.disk_bytes) }),
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
  page.innerHTML = '';
  page.append(skeleton());

  let p;
  try {
    const data = await api.get('/api/projects/' + id);
    p = data.project;
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

  const actions = h('div', { class: 'page-actions' }, [
    h('button', { class: 'btn', onclick: refreshDetail }, ICONS.refresh + ' Обновить'),
    h('button', { class: 'btn', disabled: !p.enabled, title: p.enabled ? '' : 'Проект выключен', onclick: () => window.open(p.site_url, '_blank') }, ICONS.external + ' Открыть сайт'),
    h('button', { class: 'btn', onclick: () => { window.location = api.url('/api/projects/' + p.id + '/download'); } }, ICONS.download + ' Скачать'),
    h('label', { class: 'btn btn-primary' }, [
      ICONS.upload, 'Загрузить ZIP',
      h('input', {
        type: 'file', accept: '.zip', style: 'display:none',
        onchange: (e) => uploadZip(p, e.target.files[0]),
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
        infoItem('Размер на диске', formatBytes(p.disk_bytes)),
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
    h('div', { class: 'card', id: 'metrics-card' }, [h('div', { class: 'card-head' }, [h('div', { class: 'card-title', html: 'Метрики' })]), h('div', { id: 'metrics-body' }, [skeleton()])]),
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

  loadMetrics(p);
  loadFiles(p);
}

function infoItem(label, value) {
  return h('div', { class: 'info-item' }, [
    h('div', { class: 'info-label', html: label }),
    h('div', { class: 'info-value' }, value == null || value === '' ? '—' : String(value)),
  ]);
}

async function uploadZip(p, file) {
  if (!file) return;
  const fd = new FormData();
  fd.append('file', file);
  try {
    const res = await api.upload('/api/projects/' + p.id + '/upload', fd);
    toast('Архив загружен: ' + formatBytes(res.written) + ', файлы распакованы');
    render();
  } catch (e) {
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
      [h('button', { class: 'btn btn-primary', onclick: closeModal }, 'Закрыть')]);
    if (btn) btn.disabled = false;
  }
}

async function loadMetrics(p) {
  const body = document.getElementById('metrics-body');
  try {
    const data = await api.get('/api/projects/' + p.id + '/metrics');
    const series = data.series || [];
    const labels = series.map((m) => m.ts.slice(11, 16));
    const diskGB = series.map((m) => +(m.disk_bytes / 1073741824).toFixed(3));
    const reqMin = [];
    series.forEach((m, i) => {
      if (i === 0) reqMin.push(0);
      else reqMin.push(Math.max(0, m.requests_cum - series[i - 1].requests_cum));
    });

    const c1 = h('canvas', {});
    const c2 = h('canvas', {});
    body.innerHTML = '';
    body.append(h('div', { class: 'card-pad' }, [
      h('div', { class: 'chart-legend' }, [h('span', {}, [h('span', { class: 'dot', style: 'background:#2F6FED' }), 'Диск, ГБ'])]),
      h('div', { class: 'chart-box' }, [c1]),
      h('div', { class: 'chart-legend mt-16' }, [h('span', {}, [h('span', { class: 'dot', style: 'background:#12B76A' }), 'Запросы за интервал'])]),
      h('div', { class: 'chart-box' }, [c2]),
      h('div', { class: 'mt-16 muted', html: 'Всего запросов: ' + formatNumber(data.requests_cum) + ' · Трафик: ' + formatBytes(data.bandwidth_cum) }),
    ]));
    setTimeout(() => {
      chart(c1, labels, [diskGB], { format: (v) => v.toFixed(2), height: 140 });
      chart(c2, labels, [reqMin], { format: (v) => Math.round(v), height: 140, colors: ['#12B76A'] });
    }, 20);
  } catch (e) {
    body.innerHTML = '';
    body.append(h('div', { class: 'card-pad muted' }, 'Метрики недоступны: ' + e.message));
  }
}

async function loadFiles(p) {
  const body = document.getElementById('files-body');
  try {
    const data = await api.get('/api/projects/' + p.id + '/files');
    body.innerHTML = '';
    if (!data.files || data.files.length === 0) {
      body.append(h('div', { class: 'empty-state', html: 'Файлы не загружены. Загрузите ZIP-архив с index.html.' }));
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

function renderSettings(page) {
  page.innerHTML = '';
  page.append(breadcrumbs([{ href: '#/', label: 'Панель' }, { label: 'Настройки' }]));
  page.append(h('div', { class: 'page-head' }, [
    h('div', {}, [
      h('div', { class: 'page-title', html: 'Настройки' }),
      h('div', { class: 'page-sub', html: 'Смена пароля учётной записи' }),
    ]),
  ]));

  page.append(h('div', { class: 'card', style: 'max-width:480px' }, [
    h('div', { class: 'card-head' }, [h('div', { class: 'card-title', html: 'Пароль' })]),
    h('div', { class: 'card-pad' }, [
      h('div', { class: 'field' }, [
        h('label', { html: 'Текущий пароль' }),
        h('input', { class: 'input', type: 'password', id: 'pw-cur', autocomplete: 'current-password' }),
      ]),
      h('div', { class: 'field' }, [
        h('label', { html: 'Новый пароль' }),
        h('input', { class: 'input', type: 'password', id: 'pw-new', autocomplete: 'new-password' }),
      ]),
      h('button', { class: 'btn btn-primary', onclick: async () => {
        try {
          await api.put('/api/users/me/password', {
            current_password: document.getElementById('pw-cur').value,
            new_password: document.getElementById('pw-new').value,
          });
          toast('Пароль изменён');
          document.getElementById('pw-cur').value = '';
          document.getElementById('pw-new').value = '';
        } catch (e) {
          toast(e.message, 'err');
        }
      } }, 'Сменить пароль'),
    ]),
  ]));
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
            h('td', { class: 'num', html: formatBytes(p.disk_bytes) }),
            h('td', { class: 'muted', html: formatDate(p.updated_at) }),
            h('td', { class: 'mono muted', html: escapeHtml(p.path || '') }),
          ])
        )),
      ]),
    ]),
  ]));
}

boot();
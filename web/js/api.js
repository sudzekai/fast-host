const API_BASE = 'https://api.sudzekai.ru/fast-host';

const api = {
  base: API_BASE,
  url(path) {
    return API_BASE + path;
  },
  async request(method, path, body) {
    const opts = { method, credentials: 'include', headers: {} };
    if (body instanceof FormData) {
      opts.body = body;
    } else if (body !== undefined && body !== null) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    const res = await fetch(this.url(path), opts);
    let data = null;
    try { data = await res.json(); } catch (e) { data = null; }
    if (res.status === 401) {
      throw new Error((data && data.error) || 'требуется авторизация');
    }
    if (!res.ok) {
      throw new Error((data && data.error) || ('HTTP ' + res.status));
    }
    return data;
  },
  get(p) { return this.request('GET', p); },
  post(p, b) { return this.request('POST', p, b); },
  put(p, b) { return this.request('PUT', p, b); },
  del(p) { return this.request('DELETE', p); },
  upload(p, formData) { return this.request('POST', p, formData); },
};
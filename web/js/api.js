const API_BASE = 'https://api.sudzekai.ru/fast-host';

const api = {
  base: API_BASE,
  url(path) {
    return API_BASE + path;
  },
  async request(method, path, body, options = {}) {
    const finish = !options.silent && typeof requestActivity === 'function' ? requestActivity() : () => {};
    try {
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
      const error = new Error((data && data.error) || 'требуется авторизация'); error.status = 401; throw error;
    }
    if (!res.ok) {
      throw new Error((data && data.error) || ('HTTP ' + res.status));
    }
    return data;
    } finally { finish(); }
  },
  get(p, options) { return this.request('GET', p, undefined, options); },
  post(p, b) { return this.request('POST', p, b); },
  put(p, b) { return this.request('PUT', p, b); },
  del(p) { return this.request('DELETE', p); },
  async download(path, filename) {
    const finish = requestActivity();
    try {
      const response = await fetch(this.url(path), { credentials: 'include' });
      if (!response.ok) {
        let error; try { error = await response.json(); } catch (_) {}
        throw new Error((error && error.error) || ('Ошибка скачивания: HTTP ' + response.status));
      }
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a'); link.href = url; link.download = filename;
      document.body.append(link); link.click(); link.remove();
      setTimeout(() => URL.revokeObjectURL(url), 60000);
    } finally { finish(); }
  },
  upload(p, formData, onProgress) {
    return new Promise((resolve, reject) => {
      const finish = typeof requestActivity === 'function' ? requestActivity() : () => {};
      const xhr = new XMLHttpRequest();
      xhr.addEventListener('loadend', finish);
      xhr.open('POST', this.url(p));
      xhr.withCredentials = true;
      if (onProgress && xhr.upload) {
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable && e.total > 0) onProgress(e.loaded / e.total);
        };
        xhr.upload.onload = () => onProgress(1);
      }
      xhr.onload = () => {
        let data = null;
        try { data = JSON.parse(xhr.responseText); } catch (e) { data = null; }
        if (xhr.status === 401) {
          reject(new Error((data && data.error) || 'требуется авторизация'));
          return;
        }
        if (xhr.status < 200 || xhr.status >= 300) {
          reject(new Error((data && data.error) || ('HTTP ' + xhr.status)));
          return;
        }
        resolve(data);
      };
      xhr.onerror = () => reject(new Error('сетевая ошибка при загрузке архива'));
      xhr.onabort = () => reject(new Error('загрузка отменена'));
      xhr.send(formData);
    });
  },
};

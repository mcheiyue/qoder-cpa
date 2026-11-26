(function () {
  'use strict';

  var KEY_LS = 'qoder-cpa-mgmt-key';
  var PANEL_STORE_KEY = 'cli-proxy-auth';
  var PANEL_KEY = 'managementKey';
  var PANEL_PREFIX = 'enc::v1::';
  var PANEL_SALT = 'cli-proxy-api-webui::secure-storage';
  var API_BASE = '/v0/management';
  var keyVersion = 0;

  class KeyMissing extends Error {
    constructor() {
      super('未检测到管理密钥，请在“连接与诊断”中填入 CPA Management Key 并保存。');
      this.name = 'KeyMissing';
    }
  }

  class StaleIdentity extends Error {
    constructor() {
      super('管理密钥已变更');
      this.name = 'StaleIdentity';
    }
  }

  function panelDeobfuscate(raw) {
    if (typeof raw !== 'string' || raw.indexOf(PANEL_PREFIX) !== 0) return raw;
    var secret = new TextEncoder().encode(PANEL_SALT + '|' + location.host + '|' + navigator.userAgent);
    var binary = atob(raw.slice(PANEL_PREFIX.length));
    var bytes = new Uint8Array(binary.length);
    for (var i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i) ^ secret[i % secret.length];
    return new TextDecoder().decode(bytes);
  }

  function panelStoredValue(name) {
    try {
      var raw = localStorage.getItem(name);
      return raw ? JSON.parse(panelDeobfuscate(raw)) : null;
    } catch (error) {
      return null;
    }
  }

  function borrowedKey() {
    var session = panelStoredValue(PANEL_STORE_KEY);
    var fromSession = session && session.state && session.state.managementKey;
    if (typeof fromSession === 'string' && fromSession.trim()) return fromSession.trim();
    var stored = panelStoredValue(PANEL_KEY);
    if (typeof stored === 'string' && stored.trim()) return stored.trim();
    var names = ['cliproxyapi-management-key', 'management-key', 'cpa-management-key', 'remote-management-key', 'managementKey', 'mgmt_key'];
    for (var i = 0; i < names.length; i++) {
      var value = localStorage.getItem(names[i]);
      if (value && value.length > 4 && value.indexOf('$2') !== 0) return value;
    }
    return '';
  }

  function storedKey() {
    try {
      return (localStorage.getItem(KEY_LS) || '').trim();
    } catch (error) {
      return '';
    }
  }

  function managementKey() {
    return storedKey() || borrowedKey();
  }

  function bumpVersion() {
    keyVersion += 1;
  }

  async function api(path, options) {
    var request = options || {};
    var key = managementKey();
    if (!key) throw new KeyMissing();
    var version = keyVersion;
    var headers = new Headers(request.headers || {});
    headers.set('Authorization', 'Bearer ' + key);
    headers.set('X-Management-Key', key);
    var response;
    try {
      response = await fetch(API_BASE + path, Object.assign({}, request, { headers: headers }));
    } catch (error) {
      throw new Error('网络请求失败，请检查 CPA 连接');
    }
    var body = null;
    try {
      body = await response.json();
    } catch (error) {
      body = null;
    }
    if (version !== keyVersion) throw new StaleIdentity();
    if (!response.ok) throw new Error((body && body.error) || ('请求失败，HTTP ' + response.status));
    return body || {};
  }

  function saveKey(value) {
    var trimmed = String(value || '').trim();
    if (!trimmed) throw new Error('请输入有效的 Management Key');
    localStorage.setItem(KEY_LS, trimmed);
    bumpVersion();
  }

  function clearKey() {
    try {
      localStorage.removeItem(KEY_LS);
    } catch (error) {
      /* storage may be unavailable; the in-memory state still resets */
    }
    bumpVersion();
  }

  window.QoderAPI = {
    api: api,
    managementKey: managementKey,
    storedKey: storedKey,
    borrowedKey: borrowedKey,
    saveKey: saveKey,
    clearKey: clearKey,
    KeyMissing: KeyMissing,
    StaleIdentity: StaleIdentity
  };
}());

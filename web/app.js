(function () {
  'use strict';

  var $ = function (id) { return document.getElementById(id); };
  var API = window.QoderAPI;
  var V = window.QoderViews;
  var THEME_LS = 'qoder-cpa-theme';
  var VIEWS = ['accounts', 'models', 'activity', 'connection'];
  var viewRoot = $('view-root');
  var statusEl = $('status');
  var loadedAt = $('loaded-at');
  var themeSelect = $('theme-select');

  var state = {
    view: 'accounts',
    themeMode: 'system',
    accounts: [],
    accountsState: 'loading',
    accountsError: '',
    accountsSync: '尚未刷新',
    accountSearch: '',
     models: [],
     modelsState: 'idle',
     modelsSync: '',
     modelSearch: '',
    freeOnly: false,
    selectedAuth: '',
    campaigns: null,
    campaignsState: 'idle',
    activity: null,
    activityState: 'idle',
    claimText: '等待操作',
    drafts: new Map(),
    dialogAuth: '',
    deleteName: '',
    lastAction: '等待请求'
  };
  var accountsGeneration = 0;
  var modelsGeneration = 0;
  var activityGeneration = 0;
  var dialogGeneration = 0;
  var pollGeneration = 0;
  var pollTimer = 0;
  var pollDeadline = 0;

  function setStatus(text) {
    statusEl.textContent = text || '';
  }

  function setDiagnostic(text) {
    state.lastAction = text;
    var target = $('diagnostic-status');
    if (target) target.textContent = text;
  }

  function redirectIfKeyMissing(error) {
    if (!(error instanceof API.KeyMissing)) return false;
    setView('connection');
    updateKeyUI();
    setStatus(error.message);
    return true;
  }

  function setView(view) {
    state.view = VIEWS.indexOf(view) >= 0 ? view : 'accounts';
    if (state.view !== 'activity') closeLoginDialog();
    setStatus('');
    history.replaceState(null, '', '#' + state.view);
    render();
    if (state.view === 'models' && state.modelsState === 'idle') loadModels();
  }

  function renderNav() {
    document.querySelectorAll('[data-view]').forEach(function (item) {
      var selected = item.dataset.view === state.view;
      item.classList.toggle('is-active', selected);
      item.setAttribute('aria-current', selected ? 'page' : 'false');
    });
  }

  function render() {
    renderNav();
    if (state.view === 'models') renderModelsPage();
    else if (state.view === 'activity') renderActivityPage();
    else if (state.view === 'connection') renderConnectionPage();
    else renderAccountsPage();
    renderAccountDialogIfNeeded();
  }

  function pageHead(title, description, actions) {
    return '<header class="page-head"><div><h1 id="page-title">' + title + '</h1><p>' + description + '</p></div>' +
      (actions ? '<div class="head-actions">' + actions + '</div>' : '') + '</header>';
  }

  function renderAccountsPage() {
    viewRoot.innerHTML = '<section class="page" aria-labelledby="page-title">' +
      pageHead('账号', '管理 Qoder 账号的登录状态、套餐与剩余额度。',
        '<button class="button" type="button" id="refresh">刷新状态</button>' +
        '<button class="button primary" type="button" id="login">添加账号</button>') +
      '<div id="accounts-overview">' + V.overview(state.accounts, state.accountsState, state.accountsSync) + '</div>' +
      '<div class="section"><div class="section-head"><div><h2>账号列表</h2><p>点击“编辑”查看详情并调整传输方式或优先级。</p></div>' +
      '<div class="section-head-actions"><label class="search-wrap"><svg aria-hidden="true" viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></svg>' +
      '<input id="account-search" type="search" value="' + V.escapeHTML(state.accountSearch) + '" placeholder="查找名称、邮箱或标识" aria-label="查找账号"></label></div></div>' +
      '<div id="account-list">' + accountListMarkup() + '</div>' +
      '<p class="section-note">优先级数字越大越优先；额度缺失或刷新失败显示为未读取，不会按 0 处理。</p></div></section>';
    var refresh = $('refresh');
    if (refresh) refresh.onclick = function () { loadAccounts(); };
    var login = $('login');
    if (login) login.onclick = openLoginDialog;
    var search = $('account-search');
    if (search) search.addEventListener('input', onAccountSearch);
  }

  function accountListMarkup() {
    if (state.accountsState === 'error') {
      return '<div class="empty"><strong>账号读取失败</strong><span id="accounts-error">' + V.escapeHTML(state.accountsError || '请刷新重试') + '</span></div>';
    }
    if (state.accountsState === 'loading' && !state.accounts.length) {
      return '<div class="empty"><strong>正在读取账号</strong><span>从 CPA 读取账号与配额。</span></div>';
    }
    return V.accountList(state.accounts, state.accountSearch);
  }

  function updateAccountsOverview() {
    var target = $('accounts-overview');
    if (target) target.innerHTML = V.overview(state.accounts, state.accountsState, state.accountsSync);
  }

  function updateAccountList() {
    var target = $('account-list');
    if (target) target.innerHTML = accountListMarkup();
  }

  function onAccountSearch(event) {
    state.accountSearch = event.target.value;
    updateAccountList();
  }

  async function loadAccounts() {
    var generation = ++accountsGeneration;
    var button = $('refresh');
    if (button) button.disabled = true;
    state.accountsState = 'loading';
    updateAccountsOverview();
    setStatus('正在读取账号状态和配额…');
    try {
      var body = await API.api('/qoder/accounts');
      if (generation !== accountsGeneration) return;
      state.accounts = Array.isArray(body.accounts) ? body.accounts : [];
      state.accountsState = 'ready';
      state.accountsError = '';
      state.accountsSync = '更新于 ' + new Date().toLocaleTimeString();
      loadedAt.textContent = state.accountsSync;
      var selectionChanged = reconcileSelectedAuth();
      if (state.view === 'accounts') { updateAccountList(); updateAccountsOverview(); }
      if (state.view === 'activity') {
        if ($('g1-auth')) { populateActivitySelect(); if (selectionChanged) { renderActivityBody(); enableActivityActions(); } }
        else renderActivityPage();
      }
      setDiagnostic('账号刷新完成');
      setStatus('账号状态和配额已更新。');
      reconcileAccountDialog();
    } catch (error) {
      if (error instanceof API.StaleIdentity) return;
      if (generation !== accountsGeneration) return;
      state.accountsState = 'error';
      state.accountsError = error.message;
      state.accounts = [];
      if (state.view === 'accounts') { updateAccountList(); updateAccountsOverview(); }
      reconcileAccountDialog();
      if (redirectIfKeyMissing(error)) return;
      setStatus(error.message);
      setDiagnostic(error.message);
    } finally {
      if (generation === accountsGeneration) {
        button = $('refresh');
        if (button) button.disabled = false;
      }
    }
  }

  function reconcileSelectedAuth() {
    if (state.accounts.some(function (account) { return account.auth_index === state.selectedAuth; })) return false;
    state.selectedAuth = state.accounts.length ? state.accounts[0].auth_index : '';
    invalidateActivity();
    return true;
  }

  function invalidateActivity() {
    activityGeneration += 1;
    state.campaigns = null;
    state.campaignsState = 'idle';
    state.activity = null;
    state.activityState = 'idle';
    state.claimText = '等待操作';
  }

  function renderModelsPage() {
    var merged = V.mergeModels(state.models);
    var statusText = state.modelsState === 'ready' ? state.modelsSync : state.modelsState === 'error' ? '刷新失败' : state.modelsState === 'loading' ? '正在读取模型目录' : '尚未刷新';
    viewRoot.innerHTML = '<section class="page" aria-labelledby="page-title">' +
      pageHead('模型', '查看可用模型的调用 ID，对比国际站与国内站的倍率。',
        '<button class="button" type="button" id="refresh-models">刷新目录</button>') +
      '<div class="section"><div class="section-head"><div><h2>模型目录</h2><p><span id="models-status">' + V.escapeHTML(statusText) + '</span> · <span id="models-count-label">' + modelsCountMarkup(merged.length) + '</span></p></div>' +
      '<div class="section-head-actions"><label class="search-wrap"><svg aria-hidden="true" viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></svg>' +
      '<input id="model-search" type="search" value="' + V.escapeHTML(state.modelSearch) + '" placeholder="查找模型" aria-label="查找模型"></label>' +
      '<button class="filter-button' + (state.freeOnly ? ' is-active' : '') + '" type="button" id="free-filter" aria-pressed="' + (state.freeOnly ? 'true' : 'false') + '">仅免费</button></div></div>' +
      '<div class="table-wrap"><table class="data-table"><thead><tr><th scope="col">调用 ID</th><th scope="col">展示名</th><th scope="col">全球</th><th scope="col">国内</th><th scope="col">操作</th></tr></thead>' +
      '<tbody id="models-body">' + V.modelsRows(merged, state.modelSearch, state.freeOnly, state.modelsState) + '</tbody></table></div>' +
      '<p class="section-note">倍率来自上游 price_factor，同一模型合并为一行，按区域分列；— 表示该区域无此模型。</p></div></section>';
    $('refresh-models').onclick = function () { loadModels(); };
    $('free-filter').onclick = function () {
      state.freeOnly = !state.freeOnly;
      renderModelsRows();
      $('free-filter').classList.toggle('is-active', state.freeOnly);
      $('free-filter').setAttribute('aria-pressed', state.freeOnly ? 'true' : 'false');
    };
    $('model-search').addEventListener('input', function (event) {
      state.modelSearch = event.target.value;
      renderModelsRows();
    });
  }

  function modelsCountMarkup(count) {
    if (state.modelsState === 'ready') return '共 <strong id="models-count">' + count + '</strong> 个调用 ID';
    if (state.modelsState === 'loading') return '正在读取';
    if (state.modelsState === 'error') return '未读取';
    return '尚未读取';
  }

  function renderModelsRows() {
    var merged = V.mergeModels(state.models);
    var body = $('models-body');
    if (body) body.innerHTML = V.modelsRows(merged, state.modelSearch, state.freeOnly, state.modelsState);
    var label = $('models-count-label');
    if (label) label.innerHTML = modelsCountMarkup(merged.length);
  }

  async function loadModels() {
    var generation = ++modelsGeneration;
    var button = $('refresh-models');
    if (button) button.disabled = true;
    state.modelsState = 'loading';
    if (state.view === 'models') {
      var label = $('models-status');
      if (label) label.textContent = '正在读取模型目录';
      renderModelsRows();
    }
    try {
      var body = await API.api('/qoder/models');
      if (generation !== modelsGeneration) return;
      state.models = Array.isArray(body.models) ? body.models.slice().sort(function (left, right) {
        return String(left.id).localeCompare(String(right.id)) || String(left.profile || '').localeCompare(String(right.profile || ''));
      }) : [];
      state.modelsState = 'ready';
      state.modelsSync = '更新于 ' + new Date().toLocaleTimeString();
      if (state.view === 'models') {
        renderModelsRows();
        label = $('models-status');
        if (label) label.textContent = state.modelsSync;
      }
      setDiagnostic('模型目录刷新完成');
    } catch (error) {
      if (error instanceof API.StaleIdentity) return;
      if (generation !== modelsGeneration) return;
      state.modelsState = 'error';
      state.models = [];
      if (state.view === 'models') {
        renderModelsRows();
        label = $('models-status');
        if (label) label.textContent = '刷新失败';
      }
      if (redirectIfKeyMissing(error)) return;
      setStatus(error.message);
      setDiagnostic(error.message);
    } finally {
      if (generation === modelsGeneration) {
        button = $('refresh-models');
        if (button) button.disabled = false;
      }
    }
  }

  function renderActivityPage() {
    var accounts = state.accounts;
    var hasAccounts = accounts.length > 0;
    viewRoot.innerHTML = '<section class="page" aria-labelledby="page-title">' +
      pageHead('活动', '查看选中账号的活动权益与活跃记录。',
        (hasAccounts ? '<select id="g1-auth" aria-label="活动账号">' + V.accountOptions(accounts, state.selectedAuth) + '</select>' : '') +
        '<button class="button" type="button" id="refresh-campaigns"' + (hasAccounts ? '' : ' disabled') + '>刷新活动</button>' +
        '<button class="button primary" type="button" id="fetch-activity"' + (hasAccounts ? '' : ' disabled') + '>查询活跃</button>') +
      '<div class="section"><div class="section-head"><div><h2>活动权益</h2><p id="g1-status">' + (hasAccounts ? V.escapeHTML(activityStatusText()) : '尚未查询') + '</p></div></div>' +
      '<div class="table-wrap"><table class="data-table"><thead><tr><th scope="col">活动</th><th scope="col">活动 ID</th><th scope="col">状态</th><th scope="col">权益</th><th scope="col">操作</th></tr></thead>' +
      '<tbody id="campaigns">' + campaignsMarkup() + '</tbody></table></div></div>' +
      '<div class="section"><div class="stat-strip">' +
      '<div><span>Credits 汇总</span><strong id="g1-credits">' + V.escapeHTML(creditsText()) + '</strong></div>' +
      '<div><span>活跃天数</span><strong id="g1-seat">' + V.escapeHTML(seatText()) + '</strong></div>' +
      '<div><span>领取结果</span><strong id="g1-claim">' + V.escapeHTML(state.claimText) + '</strong></div>' +
      '</div></div></section>';

    var select = $('g1-auth');
    if (select) select.addEventListener('change', function (event) {
      state.selectedAuth = event.target.value;
      invalidateActivity();
      renderActivityBody();
      enableActivityActions();
    });
    $('refresh-campaigns').onclick = function () { loadCampaigns(); };
    $('fetch-activity').onclick = function () { loadActivity(); };
  }

  function activityStatusText() {
    if (state.campaignsState === 'ready') return '更新于 ' + state.campaignsSync;
    if (state.campaignsState === 'error') return '刷新失败';
    if (state.campaignsState === 'loading') return '正在刷新活动';
    return '尚未查询';
  }

  function creditsText() {
    if (state.activityState === 'ready' && state.activity) {
      return V.formatNumber(state.activity.total_credits) + ' 总量 / ' + V.formatNumber(state.activity.peak_credits) + ' 峰值';
    }
    return state.activityState === 'error' ? '查询失败' : '尚未查询';
  }

  function seatText() {
    if (state.activityState === 'ready' && state.activity) {
      var activity = state.activity;
      return '累计 ' + V.formatNumber(activity.cumulative_days) + ' 天 · 连续 ' + V.formatNumber(activity.current_streak) +
        ' 天（最高 ' + V.formatNumber(activity.max_streak) + ' 天）· 最后 ' + (activity.last_active_date || '未记录');
    }
    return state.activityState === 'error' ? '查询失败' : '尚未查询';
  }

  function campaignsMarkup() {
    if (state.campaignsState === 'error') return '<tr><td colspan="5" class="empty">活动刷新失败</td></tr>';
    if (state.campaignsState === 'loading') return '<tr><td colspan="5" class="empty">正在刷新活动</td></tr>';
    if (!state.campaigns) return '<tr><td colspan="5" class="empty">尚未刷新</td></tr>';
    return V.campaignsRows(state.campaigns);
  }

  function enableActivityActions() {
    var hasAccounts = state.accounts.length > 0;
    ['refresh-campaigns', 'fetch-activity'].forEach(function (id) {
      var button = $(id);
      if (button) button.disabled = !hasAccounts;
    });
  }

  function populateActivitySelect() {
    var select = $('g1-auth');
    if (!select) return;
    var previous = select.value || state.selectedAuth;
    select.innerHTML = V.accountOptions(state.accounts, state.selectedAuth);
    if (previous && select.value !== previous) state.selectedAuth = select.value;
  }

  function renderActivityBody() {
    var campaigns = $('campaigns');
    if (campaigns) campaigns.innerHTML = campaignsMarkup();
    var status = $('g1-status');
    if (status) status.textContent = state.accounts.length ? activityStatusText() : '尚未查询';
    var credits = $('g1-credits');
    if (credits) credits.textContent = creditsText();
    var seat = $('g1-seat');
    if (seat) seat.textContent = seatText();
    var claim = $('g1-claim');
    if (claim) claim.textContent = state.claimText;
    var select = $('g1-auth');
    if (select) select.value = state.selectedAuth;
  }

  async function loadCampaigns() {
    var auth = state.selectedAuth;
    if (!auth) { setActivityStatus('无可用账号'); return; }
    var generation = activityGeneration;
    var button = $('refresh-campaigns');
    if (button) button.disabled = true;
    state.campaignsState = 'loading';
    renderActivityBody();
    try {
      var body = await API.api('/qoder/accounts/campaigns/refresh', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ auth_index: auth })
      });
      if (generation !== activityGeneration || auth !== state.selectedAuth) return;
      state.campaigns = Array.isArray(body.campaigns) ? body.campaigns : [];
      state.campaignsState = 'ready';
      state.campaignsSync = new Date().toLocaleTimeString();
      renderActivityBody();
      setDiagnostic('活动已刷新');
    } catch (error) {
      if (error instanceof API.StaleIdentity) return;
      if (generation !== activityGeneration || auth !== state.selectedAuth) return;
      if (redirectIfKeyMissing(error)) return;
      state.campaignsState = 'error';
      renderActivityBody();
      setStatus(error.message);
      setDiagnostic(error.message);
    } finally {
      if (generation === activityGeneration && auth === state.selectedAuth) {
        button = $('refresh-campaigns');
        if (button) button.disabled = false;
      }
    }
  }

  async function claimCampaign(campaignId, button) {
    var auth = state.selectedAuth;
    if (!auth) return;
    var generation = activityGeneration;
    if (button) button.disabled = true;
    try {
      var body = await API.api('/qoder/accounts/campaigns/claim', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ auth_index: auth, campaign_id: campaignId })
      });
      if (generation !== activityGeneration || auth !== state.selectedAuth) return;
      var label = claimOutcomeLabel(body);
      state.claimText = label + (body.grant_id ? ' · ' + body.grant_id : '');
      renderActivityBody();
      setStatus('领取结果：' + label);
      setDiagnostic('领取结果：' + label);
      await loadCampaigns();
    } catch (error) {
      if (error instanceof API.StaleIdentity) return;
      if (generation !== activityGeneration || auth !== state.selectedAuth) return;
      state.claimText = '领取失败：' + error.message;
      renderActivityBody();
      setStatus(error.message);
    } finally {
      if (button && generation === activityGeneration && auth === state.selectedAuth) button.disabled = false;
    }
  }

  function claimOutcomeLabel(body) {
    return ({ claimed: '已领取', replayed: '已领取过（幂等）', expired: '活动已过期', rejected: '领取被拒绝' })[body.outcome] || body.outcome || '-';
  }

  function setActivityStatus(text) {
    var status = $('g1-status');
    if (status) status.textContent = text;
  }

  async function loadActivity() {
    var auth = state.selectedAuth;
    if (!auth) { setActivityStatus('无可用账号'); return; }
    var generation = activityGeneration;
    var button = $('fetch-activity');
    if (button) button.disabled = true;
    state.activityState = 'loading';
    renderActivityBody();
    try {
      var body = await API.api('/qoder/accounts/activity/fetch', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ auth_index: auth })
      });
      if (generation !== activityGeneration || auth !== state.selectedAuth) return;
      state.activity = body;
      state.activityState = 'ready';
      renderActivityBody();
      setDiagnostic('活跃记录已查询');
    } catch (error) {
      if (error instanceof API.StaleIdentity) return;
      if (generation !== activityGeneration || auth !== state.selectedAuth) return;
      if (redirectIfKeyMissing(error)) return;
      state.activityState = 'error';
      renderActivityBody();
      setStatus(error.message);
      setDiagnostic(error.message);
    } finally {
      if (generation === activityGeneration && auth === state.selectedAuth) {
        button = $('fetch-activity');
        if (button) button.disabled = false;
      }
    }
  }

  function renderConnectionPage() {
    var stored = API.storedKey();
    var borrowed = !stored && API.borrowedKey();
    var title = stored ? '已保存管理密钥' : borrowed ? '正在借用 CPA 面板密钥' : '未连接';
    var text = stored
      ? '密钥保存在本机浏览器，请求只发往同源 /v0/management。'
      : borrowed
        ? '检测到 CPA 面板已保存的密钥；也可以在这里改为插件自留密钥。'
        : '填入 CPA Management Key 后才能读取账号、模型与活动。';
    viewRoot.innerHTML = '<section class="page" aria-labelledby="page-title">' +
      pageHead('连接与诊断', '管理密钥只保存在本机浏览器，不写入 URL 或日志。', '') +
      '<div class="connection-grid">' +
      '<div class="section"><div class="connection-panel"><svg aria-hidden="true" viewBox="0 0 24 24"><path d="M8.5 12h7m-10.5 0a3.5 3.5 0 1 0 0 7h3m8-14h-3a3.5 3.5 0 1 0 0 7h3a3.5 3.5 0 1 0 0-7Z"/></svg>' +
      '<div><h2 id="key-state-title">' + V.escapeHTML(title) + '</h2><p id="key-state-text">' + V.escapeHTML(text) + '</p></div></div>' +
      '<form class="connection-form" id="key-form"><label class="field" for="mgmt-key"><span>CPA Management Key</span>' +
      '<input type="password" id="mgmt-key" value="' + V.escapeHTML(stored) + '" placeholder="输入 CPA Management Key" autocomplete="off" spellcheck="false">' +
      '<small id="key-hint"></small></label>' +
      '<div class="field-actions"><button class="button primary" type="submit" id="save-key">连接并保存</button>' +
      '<button class="button ghost" type="button" id="clear-key"' + (stored ? '' : ' hidden') + '>清除密钥</button></div></form></div>' +
      '<div class="section"><div class="section-head"><div><h2>诊断摘要</h2><p>不保存请求体或凭据</p></div></div>' +
      '<ul class="diagnostic-list"><li><span>执行协议</span><strong>Chat Completions，CPA 内部格式兼容 openai</strong></li>' +
      '<li><span>上游错误</span><strong>透传 HTTP 状态与脱敏 code/type/message</strong></li>' +
      '<li><span>管理接口</span><strong>/v0/management</strong></li>' +
      '<li><span>最近操作</span><strong id="diagnostic-status">' + V.escapeHTML(state.lastAction) + '</strong></li></ul></div>' +
      '</div></section>';
    updateKeyUI();
    $('key-form').addEventListener('submit', function (event) {
      event.preventDefault();
      var value = $('mgmt-key').value;
      try {
        API.saveKey(value);
      } catch (error) {
        setStatus(error.message);
        return;
      }
      resetForNewIdentity();
      setView('accounts');
      setStatus('管理密钥已保存，正在重新加载账号列表…');
      loadAccounts();
    });
    $('clear-key').onclick = function () {
      API.clearKey();
      resetForNewIdentity();
      setView('connection');
      setStatus('已清除管理密钥。');
    };
  }

  function updateKeyUI() {
    var hint = $('key-hint');
    if (!hint) return;
    if (API.storedKey()) hint.textContent = '已保存自留密钥';
    else if (API.borrowedKey()) hint.textContent = '已自动借用 CPA 面板密钥';
    else hint.textContent = '未连接';
  }

  function resetForNewIdentity() {
    pollGeneration += 1;
    clearInterval(pollTimer);
    pollTimer = 0;
    closeAccountDialog(false);
    closeLoginDialog(true);
    closeDeleteDialog();
    state.drafts.clear();
    state.accounts = [];
    state.accountsState = 'loading';
    state.accountsSync = '尚未刷新';
    state.accountsError = '';
    state.models = [];
    state.modelsState = 'idle';
    state.modelSearch = '';
    state.freeOnly = false;
    state.selectedAuth = '';
    state.lastAction = '等待请求';
    accountsGeneration += 1;
    modelsGeneration += 1;
    invalidateActivity();
    loadedAt.textContent = '尚未刷新';
  }

  function reconcileAccountDialog() {
    var dialog = $('account-dialog');
    if (!dialog || !dialog.open) return;
    var account = currentAccount();
    if (!account) {
      closeAccountDialog(true);
      setStatus('正在编辑的账号已不在最新列表中，编辑器已关闭。');
      return;
    }
    renderAccountDetails(account);
  }

  function renderAccountDialogIfNeeded() {
    if (!$('account-dialog').open) return;
    var account = state.accounts.find(function (item) { return item.auth_index === state.dialogAuth; });
    if (!account) { closeAccountDialog(false); return; }
    renderAccountDetails(account);
  }

  function openAccountDialog(auth) {
    var account = state.accounts.find(function (item) { return item.auth_index === auth; });
    if (!account) return;
    state.dialogAuth = auth;
    dialogGeneration += 1;
    $('account-dialog-title').textContent = account.label || account.name || '编辑账号';
    $('account-dialog-sub').textContent = (account.email || '未提供邮箱') + ' · ' + V.transportName(account.transport_profile);
    $('account-edit-status').textContent = '';
    $('account-dialog-body').innerHTML = '<div id="account-details"></div>' +
      '<div class="sheet-section"><h3>传输方式与优先级</h3>' +
      '<div class="inline-fields"><label class="field" for="edit-transport"><span>传输方式</span><select id="edit-transport">' + V.transportOptions(draftOf(account).transport_profile) + '</select><small>保存后写入账号凭证。</small></label>' +
      '<label class="field" for="edit-priority"><span>优先级</span><input id="edit-priority" type="number" inputmode="numeric" value="' + V.escapeHTML(draftOf(account).priority) + '"><small>数字越大越优先。</small></label></div>' +
      '<div class="save-row"><button class="button" type="button" id="save-transport">保存传输方式</button><button class="button" type="button" id="save-priority">保存优先级</button></div>' +
      '<p class="muted">两项分别写入宿主；单项失败不会显示为保存成功，未保存的草稿会保留。</p></div>' +
      '<div class="sheet-section"><h3>配额</h3><button class="button" type="button" id="edit-refresh">刷新配额</button>' +
      '<p class="muted">刷新会重新读取该账号配额并更新列表；当前编辑内容不会丢失。</p></div>' +
      '<div class="sheet-section danger-zone"><h3>危险操作</h3><p>删除会移除宿主凭证文件，需重新登录才能恢复。</p>' +
      '<button class="button danger" type="button" id="edit-delete">删除账号</button></div>';
    renderAccountDetails(account);
    var transport = $('edit-transport');
    var priority = $('edit-priority');
    transport.addEventListener('change', recordDraft);
    priority.addEventListener('input', recordDraft);
    $('save-transport').onclick = saveTransport;
    $('save-priority').onclick = savePriority;
    $('edit-refresh').onclick = refreshQuotaFromDialog;
    $('edit-delete').onclick = function () { requestDelete(account); };
    $('account-dialog').showModal();
    transport.focus({ preventScroll: true });
  }

  function draftOf(account) {
    var draft = state.drafts.get(account.auth_index) || {};
    return {
      transport_profile: draft.transport_profile || account.transport_profile || 'cosy-api2',
      priority: draft.priority != null ? String(draft.priority) : String(account.priority || 0)
    };
  }

  function recordDraft() {
    if (!state.dialogAuth) return;
    var draft = state.drafts.get(state.dialogAuth) || {};
    var transport = $('edit-transport');
    var priority = $('edit-priority');
    if (transport) draft.transport_profile = transport.value;
    if (priority) draft.priority = priority.value;
    state.drafts.set(state.dialogAuth, draft);
  }

  function renderAccountDetails(account) {
    var target = $('account-details');
    if (target) target.innerHTML = V.accountDetails(account);
    var sub = $('account-dialog-sub');
    if (sub) sub.textContent = (account.email || '未提供邮箱') + ' · ' + V.transportName(account.transport_profile);
  }

  function setEditStatus(text) {
    var target = $('account-edit-status');
    if (target) target.textContent = text;
  }

  function closeAccountDialog(discardDraft) {
    var dialog = $('account-dialog');
    if (discardDraft && state.dialogAuth) state.drafts.delete(state.dialogAuth);
    if (dialog && dialog.open) dialog.close();
    state.dialogAuth = '';
    dialogGeneration += 1;
  }

  async function saveTransport() {
    var button = $('save-transport');
    var select = $('edit-transport');
    if (!button || !select || button.disabled) return;
    recordDraft();
    var auth = state.dialogAuth;
    var generation = dialogGeneration;
    var listGeneration = accountsGeneration;
    var profile = select.value;
    button.disabled = true;
    setEditStatus('正在保存传输方式…');
    try {
      await API.api('/qoder/accounts/profile', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ auth_index: auth, transport_profile: profile })
      });
      var account = applyPersistedAccount(auth, listGeneration, function (item) { item.transport_profile = profile; });
      if (generation !== dialogGeneration) return;
      if (account) renderAccountDetails(account);
      setEditStatus('传输方式已保存。');
      setStatus('传输方式已保存。');
      setDiagnostic('传输方式已保存');
    } catch (error) {
      if (error instanceof API.StaleIdentity) return;
      if (generation !== dialogGeneration) return;
      setEditStatus('传输方式保存失败：' + error.message + '；草稿已保留。');
      setStatus(error.message);
      setDiagnostic(error.message);
    } finally {
      if (generation === dialogGeneration) {
        button = $('save-transport');
        if (button) button.disabled = false;
      }
    }
  }

  async function savePriority() {
    var button = $('save-priority');
    var input = $('edit-priority');
    if (!button || !input || button.disabled) return;
    recordDraft();
    var auth = state.dialogAuth;
    var generation = dialogGeneration;
    var listGeneration = accountsGeneration;
    var name = currentAccount()?.name || '';
    var priority = parseInt(input.value, 10);
    if (!Number.isInteger(priority)) {
      setEditStatus('优先级需为整数；草稿已保留。');
      return;
    }
    if (!name) {
      setEditStatus('该账号无宿主文件名，无法保存优先级；草稿已保留。');
      return;
    }
    button.disabled = true;
    setEditStatus('正在保存优先级…');
    try {
      await API.api('/auth-files/fields', {
        method: 'PATCH', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name, priority: priority })
      });
      applyPersistedAccount(auth, listGeneration, function (item) { item.priority = priority; });
      if (generation !== dialogGeneration) return;
      // 保存期间用户可能已改成新值：只有输入框仍是刚保存的值时才回写草稿，避免覆盖更新的输入。
      var latest = $('edit-priority');
      if (latest && latest.value === String(priority)) {
        state.drafts.set(auth, Object.assign({}, state.drafts.get(auth) || {}, { priority: String(priority) }));
      }
      setEditStatus('优先级已保存（数字越大越优先）。');
      setStatus('优先级已保存（数字越大越优先）。');
      setDiagnostic('优先级已保存');
    } catch (error) {
      if (error instanceof API.StaleIdentity) return;
      if (generation !== dialogGeneration) return;
      setEditStatus('优先级保存失败：' + error.message + '；草稿已保留。');
      setStatus(error.message);
      setDiagnostic(error.message);
    } finally {
      if (generation === dialogGeneration) {
        button = $('save-priority');
        if (button) button.disabled = false;
      }
    }
  }

  function currentAccount() {
    return state.accounts.find(function (item) { return item.auth_index === state.dialogAuth; }) || null;
  }

  // 保存结果属于账号本身，不属于发起保存的编辑器：身份与列表代际未变就写回模型并重绘列表。
  function applyPersistedAccount(auth, listGeneration, mutate) {
    if (accountsGeneration !== listGeneration) return null;
    var account = state.accounts.find(function (item) { return item.auth_index === auth; }) || null;
    if (!account) return null;
    mutate(account);
    if (state.view === 'accounts') updateAccountList();
    return account;
  }

  async function refreshQuotaFromDialog() {
    var button = $('edit-refresh');
    var auth = state.dialogAuth;
    if (!button || !auth || button.disabled) return;
    var generation = dialogGeneration;
    button.disabled = true;
    setEditStatus('正在刷新配额…');
    try {
      await API.api('/qoder/accounts/quota/refresh', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ auth_index: auth })
      });
      if (generation !== dialogGeneration) return;
      await loadAccounts();
      if (generation !== dialogGeneration) return;
      var account = currentAccount();
      if (account) renderAccountDetails(account);
      setEditStatus('配额已刷新。');
      setStatus('配额已刷新。');
    } catch (error) {
      if (error instanceof API.StaleIdentity) return;
      if (generation !== dialogGeneration) return;
      setEditStatus('配额刷新失败：' + error.message);
      setStatus(error.message);
    } finally {
      if (generation === dialogGeneration) {
        button = $('edit-refresh');
        if (button) button.disabled = false;
      }
    }
  }

  function requestDelete(account) {
    if (!account.name) {
      setEditStatus('该账号无宿主文件名，无法删除');
      return;
    }
    state.deleteName = account.name;
    state.deleteAuth = account.auth_index;
    $('delete-message').textContent = '确定删除账号 ' + (account.label || account.name) + '？';
    $('delete-dialog').showModal();
    $('delete-cancel').focus();
  }

  function closeDeleteDialog() {
    var dialog = $('delete-dialog');
    if (dialog && dialog.open) dialog.close();
    state.deleteName = '';
    state.deleteAuth = '';
  }

  async function confirmDelete() {
    var name = state.deleteName;
    var auth = state.deleteAuth;
    if (!name) { closeDeleteDialog(); return; }
    var button = $('delete-confirm');
    button.disabled = true;
    try {
      await API.api('/auth-files?name=' + encodeURIComponent(name), { method: 'DELETE' });
      closeDeleteDialog();
      closeAccountDialog(false);
      if (auth) state.drafts.delete(auth);
      setStatus('账号已删除。');
      setDiagnostic('账号已删除');
      await loadAccounts();
    } catch (error) {
      if (error instanceof API.StaleIdentity) return;
      closeDeleteDialog();
      setStatus(error.message);
      setDiagnostic(error.message);
    } finally {
      button.disabled = false;
    }
  }

  function openLoginDialog() {
    pollGeneration += 1;
    clearInterval(pollTimer);
    pollTimer = 0;
    $('login-status').textContent = '';
    $('login-start').disabled = false;
    $('login-start').textContent = '开始授权';
    $('login-cancel').hidden = true;
    var radios = document.querySelectorAll('input[name="login-site"]');
    if (radios.length) radios[0].checked = true;
    $('login-dialog').showModal();
    if (radios.length) radios[0].focus();
  }

  function closeLoginDialog(silent) {
    pollGeneration += 1;
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = 0;
    var dialog = $('login-dialog');
    if (dialog && dialog.open) dialog.close();
    var cancel = $('login-cancel');
    if (cancel) cancel.hidden = true;
    var start = $('login-start');
    if (start) { start.disabled = false; start.textContent = '开始授权'; }
    if (!silent) { var loginStatus = $('login-status'); if (loginStatus) loginStatus.textContent = '已停止等待授权。'; }
  }

  function stopLoginPolling(message) {
    pollGeneration += 1;
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = 0;
    var cancel = $('login-cancel');
    if (cancel) cancel.hidden = true;
    var start = $('login-start');
    if (start) { start.disabled = false; start.textContent = '开始授权'; }
    if (message) { var loginStatus = $('login-status'); if (loginStatus) loginStatus.textContent = message; }
  }

  async function startLogin() {
    var dialog = $('login-dialog');
    if (!dialog || !dialog.open) return;
    var start = $('login-start');
    var loginStatus = $('login-status');
    var generation = ++pollGeneration;
    var site = document.querySelector('input[name="login-site"]:checked');
    var region = site ? site.value : 'global';
    start.disabled = true;
    start.textContent = '等待授权…';
    loginStatus.textContent = '正在启动授权…';
    try {
      var body = await API.api('/qoder-auth-url' + (region === 'cn' ? '?region=cn' : ''));
      if (generation !== pollGeneration) return;
      if (!body.url) throw new Error(body.error || '无法启动登录');
      var loginURL;
      try {
        loginURL = new URL(body.url, location.href);
      } catch (error) {
        throw new Error('Qoder 授权地址无效');
      }
      if (loginURL.protocol !== 'http:' && loginURL.protocol !== 'https:') throw new Error('Qoder 授权地址协议不受支持');
      var href = loginURL.href;
      var popup = window.open(href, '_blank', 'noopener');
      if (popup) loginStatus.textContent = '等待 Qoder 授权…';
      else loginStatus.innerHTML = '浏览器阻止了弹窗，请<a href="' + V.escapeHTML(href) + '" target="_blank" rel="noopener">点击此处授权</a>。';
      $('login-cancel').hidden = false;
      pollDeadline = Date.now() + 300000;
      pollTimer = setInterval(async function () {
        if (generation !== pollGeneration) return;
        if (Date.now() >= pollDeadline) { stopLoginPolling('授权等待已超时，请重新发起登录。'); return; }
        try {
          var authState = body.state;
          var result = await API.api('/get-auth-status?state=' + encodeURIComponent(authState));
          if (generation !== pollGeneration) return;
          if (Date.now() >= pollDeadline) { stopLoginPolling('授权等待已超时，请重新发起登录。'); return; }
          if (result.status === 'ok') {
            stopLoginPolling('');
            closeLoginDialog(true);
            setStatus('登录成功，账号已保存。');
            setDiagnostic('登录成功');
            loadAccounts();
          } else if (result.status === 'error') {
            stopLoginPolling('登录失败：' + (result.error || '未知错误'));
          }
        } catch (error) {
          if (error instanceof API.StaleIdentity) return;
          if (generation === pollGeneration) stopLoginPolling(error.message);
        }
      }, 2500);
    } catch (error) {
      if (error instanceof API.StaleIdentity) return;
      if (generation !== pollGeneration) return;
      stopLoginPolling(error.message);
      setStatus(error.message);
    }
  }

  function applyTheme(mode) {
    var next = mode === 'light' || mode === 'dark' ? mode : 'system';
    state.themeMode = next;
    try { localStorage.setItem(THEME_LS, next); } catch (error) { /* theme persists in memory */ }
    var dark = next === 'dark' || (next === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
    document.documentElement.dataset.theme = dark ? 'dark' : 'light';
    document.documentElement.dataset.themeMode = next;
    if (themeSelect) themeSelect.value = next;
  }

  function copyModelId(value) {
    if (!navigator.clipboard || !window.isSecureContext) {
      setStatus('无法自动复制，请手动选中 ' + value + ' 后复制。');
      return;
    }
    navigator.clipboard.writeText(value).then(function () {
      setStatus('已复制模型 ID：' + value);
      setDiagnostic('已复制模型 ID');
    }, function () {
      setStatus('无法自动复制，请手动选中 ' + value + ' 后复制。');
    });
  }

  function bindGlobal() {
    document.getElementById('task-nav').addEventListener('click', function (event) {
      var target = event.target.closest('[data-view]');
      if (!target) return;
      setView(target.dataset.view);
    });
    viewRoot.addEventListener('click', function (event) {
      var edit = event.target.closest('[data-edit]');
      if (edit) { openAccountDialog(edit.dataset.edit); return; }
      var copy = event.target.closest('[data-copy-id]');
      if (copy) { copyModelId(copy.dataset.copyId); return; }
      var claim = event.target.closest('[data-claim]');
      if (claim) { claimCampaign(claim.dataset.claim, claim); }
    });
    themeSelect.addEventListener('change', function (event) { applyTheme(event.target.value); });
    window.addEventListener('hashchange', function () {
      var target = (location.hash || '').replace('#', '');
      var view = VIEWS.indexOf(target) >= 0 ? target : 'accounts';
      if (view !== state.view) setView(view);
    });
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function () {
      if (state.themeMode === 'system') applyTheme('system');
    });
    $('account-dialog-close').onclick = function () { closeAccountDialog(true); };
    $('account-dialog-cancel').onclick = function () { closeAccountDialog(true); };
    $('account-dialog').addEventListener('cancel', function (event) { event.preventDefault(); closeAccountDialog(true); });
    $('account-dialog').addEventListener('click', function (event) { if (event.target === $('account-dialog')) closeAccountDialog(true); });
    $('login-dialog-close').onclick = function () { closeLoginDialog(); };
    $('login-dialog').addEventListener('cancel', function (event) { event.preventDefault(); closeLoginDialog(); });
    $('login-dialog').addEventListener('click', function (event) { if (event.target === $('login-dialog')) closeLoginDialog(); });
    $('login-cancel').onclick = function () { stopLoginPolling('已停止等待授权。'); };
    $('login-start').onclick = startLogin;
    $('delete-cancel').onclick = closeDeleteDialog;
    $('delete-dialog').addEventListener('cancel', function (event) { event.preventDefault(); closeDeleteDialog(); });
    $('delete-dialog').addEventListener('click', function (event) { if (event.target === $('delete-dialog')) closeDeleteDialog(); });
    $('delete-confirm').onclick = confirmDelete;
  }

  function boot() {
    var saved = 'system';
    try { saved = localStorage.getItem(THEME_LS) || 'system'; } catch (error) { saved = 'system'; }
    applyTheme(saved);
    bindGlobal();
    var hash = (location.hash || '').replace('#', '');
    state.view = VIEWS.indexOf(hash) >= 0 ? hash : 'accounts';
    if (!API.managementKey()) state.view = 'connection';
    history.replaceState(null, '', '#' + state.view);
    render();
    if (state.view === 'connection') {
      setStatus('未检测到管理密钥，请先在“连接与诊断”中保存。');
      return;
    }
    if (state.view === 'models') loadModels();
    loadAccounts();
  }

  boot();
}());

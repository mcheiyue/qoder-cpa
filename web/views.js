(function () {
  'use strict';

  var TRANSPORT_LABELS = {
    'cosy-api2': '国际 API2',
    'cosy-api3': '国际 API3',
    'cosy-cn': '国内站',
    'bearer-openai': '国际 Bearer'
  };
  var TRANSPORT_ORDER = ['cosy-api2', 'cosy-api3', 'cosy-cn', 'bearer-openai'];

  function escapeHTML(value) {
    return String(value == null ? '' : value).replace(/[&<>"']/g, function (character) {
      return ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[character];
    });
  }

  function isNumber(value) {
    return typeof value === 'number' && Number.isFinite(value);
  }

  function formatNumber(value) {
    return isNumber(value) ? value.toLocaleString('zh-CN') : '-';
  }

  function formatTime(value) {
    if (!value) return '-';
    var date = new Date(value);
    return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  }

  function formatDate(value) {
    if (!value) return '';
    var date = new Date(value);
    return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString('zh-CN');
  }

  function initials(account) {
    var source = String(account.label || account.name || 'Q').replace(/账号|全球|国内|测试|备用/g, '').trim();
    return source.slice(0, 1) || 'Q';
  }

  function transportName(profile) {
    return TRANSPORT_LABELS[profile] || profile || '未提供';
  }

  function transportOptions(selected) {
    return TRANSPORT_ORDER.map(function (value) {
      return '<option value="' + value + '"' + (value === selected ? ' selected' : '') + '>' + TRANSPORT_LABELS[value] + '</option>';
    }).join('');
  }

  function statusOf(account) {
    if (account.disabled) return { label: '已停用', tone: 'muted' };
    if (account.needs_reauth) return { label: '需重新授权', tone: 'danger' };
    if (account.quota_error) return { label: '额度刷新失败', tone: 'warn' };
    if (account.exhausted) return { label: '额度耗尽', tone: 'warn' };
    if (account.quota_synced_at) return { label: '正常', tone: 'ok' };
    return { label: '未刷新', tone: 'muted' };
  }

  function badge(account) {
    var status = statusOf(account);
    return '<span class="badge ' + status.tone + '">' + escapeHTML(status.label) + '</span>';
  }

  function quotaKnown(account) {
    return Boolean(account.quota_synced_at);
  }

  function periodText(account) {
    if (!account.period_end) return '';
    var end = formatDate(account.period_end);
    if (!end || account.period_end === '9999-12-31T00:00:00Z') return '';
    return '至 ' + end;
  }

  function sharedText(shared, unit) {
    if (!shared) return '';
    if (shared.unlimited) return '组织共享不限量 · 已用 ' + formatNumber(shared.used) + ' ' + unit;
    if (!shared.available) return '组织共享不可用';
    return '组织共享 ' + formatNumber(shared.remaining) + ' / ' + formatNumber(shared.limit) + ' ' + unit + ' · 已用 ' + formatNumber(shared.used);
  }

  function sharedDetailText(shared, unit) {
    if (!shared) return '无';
    if (shared.unlimited) return '不限量 · 已用 ' + formatNumber(shared.used) + ' ' + unit;
    if (!shared.available) return '不可用';
    return formatNumber(shared.remaining) + ' / ' + formatNumber(shared.limit) + ' ' + unit + ' · 已用 ' + formatNumber(shared.used);
  }

  function quotaExtraText(account) {
    var unit = account.unit || '';
    var parts = [];
    if (isNumber(account.add_on) && account.add_on > 0) {
      parts.push('附加 ' + formatNumber(account.add_on_remaining) + ' / ' + formatNumber(account.add_on) + ' ' + unit);
    }
    var shared = sharedText(account.shared, unit);
    if (shared) parts.push(shared);
    return parts.join(' · ');
  }

  function quotaCell(account) {
    if (account.needs_reauth) {
      return '<div class="cell" data-label="额度"><strong>额度未确认</strong><small>重新授权后读取</small></div>';
    }
    if (account.quota_error) {
      return '<div class="cell" data-label="额度"><strong>额度刷新失败</strong><small>' + escapeHTML(account.quota_error) + '</small></div>';
    }
    if (!quotaKnown(account)) {
      return '<div class="cell" data-label="额度"><strong>未读取</strong><small>打开编辑面板可刷新配额</small></div>';
    }
    var unit = account.unit || '';
    var remaining = isNumber(account.remaining) ? account.remaining : 0;
    var limit = isNumber(account.limit) ? account.limit : 0;
    var percent = limit > 0 ? Math.max(0, Math.min(100, Math.round((remaining / limit) * 100))) : 0;
    var extra = quotaExtraText(account);
    var used = isNumber(account.used) ? '已用 ' + formatNumber(account.used) + ' ' + unit : '';
    var reset = account.reset_at ? '重置 ' + formatTime(account.reset_at) : '';
    var meta = [used, reset].filter(Boolean).join(' · ');
    return '<div class="cell" data-label="额度"><strong>' + formatNumber(remaining) + ' / ' + formatNumber(limit) + ' ' + escapeHTML(unit) + '</strong>' +
      (extra ? '<small>' + escapeHTML(extra) + '</small>' : '') +
      (meta ? '<small>' + escapeHTML(meta) + '</small>' : '') +
      (limit > 0 ? '<div class="meter" aria-label="剩余 ' + percent + '%"><span style="width:' + percent + '%"></span></div>' : '') +
      '</div>';
  }

  function planCell(account) {
    var tier = account.plan_tier || '未提供套餐';
    var period = periodText(account);
    var region = transportName(account.transport_profile);
    return '<div class="cell" data-label="套餐"><strong>' + escapeHTML(tier) + '</strong><small>' + escapeHTML(region) + (period ? ' · ' + escapeHTML(period) : '') + '</small></div>';
  }

  function accountRow(account) {
    var label = account.label || account.name || account.auth_index || '未命名账号';
    var email = account.email || '未提供邮箱';
    return '<article class="account-row" data-auth-row="' + escapeHTML(account.auth_index) + '">' +
      '<div class="identity"><span class="avatar" aria-hidden="true">' + escapeHTML(initials(account)) + '</span><div class="identity-copy">' +
      '<strong>' + escapeHTML(label) + '</strong><small>' + escapeHTML(email) + '</small><code>' + escapeHTML(account.auth_index || '') + '</code>' +
      '</div></div>' +
      planCell(account) +
      quotaCell(account) +
      '<div class="cell" data-label="状态">' + badge(account) + '</div>' +
      '<div class="row-action"><button class="text-button" type="button" data-edit="' + escapeHTML(account.auth_index) + '">编辑</button></div>' +
      '</article>';
  }

  function accountList(accounts, search) {
    if (!accounts.length) {
      return '<div class="empty"><strong>还没有 Qoder 账号</strong><span>点击“添加账号”完成 CPA 原生 Qoder 授权。</span></div>';
    }
    var needle = String(search || '').trim().toLowerCase();
    var rows = accounts.filter(function (account) {
      if (!needle) return true;
      return [account.label, account.name, account.auth_index, account.email, transportName(account.transport_profile)]
        .join(' ').toLowerCase().indexOf(needle) >= 0;
    });
    if (!rows.length) {
      return '<div class="empty"><strong>没有匹配的账号</strong><span>换一个名称、邮箱或标识再试。</span></div>';
    }
    return rows.map(accountRow).join('');
  }

  function overview(accounts, state, syncText) {
    if (state === 'error') {
      return '<div class="overview"><span>全部 <strong id="overview-total">-</strong></span><span>可用 <strong id="overview-available">-</strong></span><span>需处理 <strong class="attention" id="overview-attention">读取失败</strong></span><span class="sync" id="overview-sync">账号读取失败，请刷新重试</span></div>';
    }
    if (state === 'loading' && !accounts.length) {
      return '<div class="overview"><span>全部 <strong id="overview-total">-</strong></span><span>可用 <strong id="overview-available">-</strong></span><span>需处理 <strong class="attention" id="overview-attention">-</strong></span><span class="sync" id="overview-sync">正在读取账号</span></div>';
    }
    var available = accounts.filter(function (account) {
      return !account.disabled && !account.needs_reauth && !account.exhausted && !account.quota_error;
    }).length;
    var attention = accounts.length - available;
    return '<div class="overview"><span>全部 <strong id="overview-total">' + accounts.length + '</strong></span><span>可用 <strong id="overview-available">' + available + '</strong></span>' +
      '<span>需处理 <strong class="attention" id="overview-attention">' + attention + '</strong></span><span class="sync" id="overview-sync">' + escapeHTML(syncText || '') + '</span></div>';
  }

  function detailRow(label, value) {
    return '<div><dt>' + escapeHTML(label) + '</dt><dd>' + value + '</dd></div>';
  }

  function dedicatedText(account, unit) {
    if (!account.dedicated || !account.dedicated.length) return '无';
    return account.dedicated.map(function (pack) {
      var name = pack.name || '专属资源包';
      var status = pack.available ? '' : '（不可用）';
      return escapeHTML(name) + ' ' + formatNumber(pack.remaining) + ' / ' + formatNumber(pack.limit) + ' ' + escapeHTML(unit) + status;
    }).join('；');
  }

  function accountDetails(account) {
    var unit = account.unit || '';
    var status = statusOf(account);
    var known = quotaKnown(account);
    var mainQuota = known
      ? formatNumber(account.remaining) + ' / ' + formatNumber(account.limit) + ' ' + unit
      : (account.quota_error ? account.quota_error : '未读取');
    var addon = isNumber(account.add_on) && account.add_on > 0
      ? formatNumber(account.add_on_remaining) + ' / ' + formatNumber(account.add_on) + ' ' + unit
      : '无';
    var shared = sharedDetailText(account.shared, unit);
    var upgrade = safeURL(account.upgrade_url)
      ? '<a href="' + escapeHTML(safeURL(account.upgrade_url)) + '" target="_blank" rel="noopener noreferrer">打开升级页</a>'
      : '未提供';
    var period = [periodText(account), account.period_start ? '开始 ' + formatDate(account.period_start) : ''].filter(Boolean).join(' · ');
    return '<dl class="detail-list">' +
      detailRow('邮箱 / 手机', escapeHTML(account.email || '未提供')) +
      detailRow('账号标识', escapeHTML(account.auth_index || '未提供')) +
      detailRow('宿主文件名', escapeHTML(account.name || '未提供')) +
      detailRow('状态', escapeHTML(status.label + (account.status ? ' · ' + account.status : ''))) +
      detailRow('用户类型', escapeHTML(account.user_type || '未提供')) +
      detailRow('套餐', escapeHTML((account.plan_tier || '未提供') + (account.paid_plan ? '（付费）' : '')) + (period ? ' · ' + escapeHTML(period) : '')) +
      detailRow('主额度', known ? escapeHTML(mainQuota) : '<span class="badge warn">' + escapeHTML(mainQuota) + '</span>') +
      detailRow('附加额度', escapeHTML(addon)) +
      detailRow('组织共享', escapeHTML(shared)) +
      detailRow('专属资源包', dedicatedText(account, unit)) +
      detailRow('Agent 额度', isNumber(account.agent_limit) && account.agent_limit > 0 ? formatNumber(account.agent_limit) + ' ' + escapeHTML(unit) : '未提供') +
      detailRow('已用', isNumber(account.used) ? formatNumber(account.used) + ' ' + escapeHTML(unit) : '未提供') +
      detailRow('下次重置', escapeHTML(formatTime(account.reset_at))) +
      detailRow('配额同步', escapeHTML(formatTime(account.quota_synced_at))) +
      detailRow('升级链接', upgrade) +
      '</dl>';
  }

  function mergeModels(rows) {
    var byID = new Map();
    rows.forEach(function (row) {
      var id = row.id || '';
      if (!byID.has(id)) byID.set(id, { id: id, display_name: '', global: null, cn: null, free: false });
      var entry = byID.get(id);
      if (!entry.display_name) entry.display_name = row.display_name || '';
      entry.free = entry.free || Boolean(row.is_free) || row.rate === 0;
      if (row.profile === 'cosy-cn') entry.cn = row;
      else entry.global = row;
    });
    return Array.from(byID.values()).sort(function (left, right) { return left.id.localeCompare(right.id); });
  }

  function rateCell(model) {
    if (!model) return '<span class="muted">—</span>';
    if (model.is_free || model.rate === 0) return '<span class="badge ok">免费</span>';
    var rate = '<code>x' + escapeHTML(Number(model.rate).toFixed(1)) + '</code>';
    return model.badge ? rate + ' <span class="badge accent">' + escapeHTML(model.badge) + '</span>' : rate;
  }

  function safeURL(value) {
    var text = String(value == null ? '' : value).trim();
    if (!text) return '';
    var parsed;
    try {
      parsed = new URL(text, location.href);
    } catch (error) {
      return '';
    }
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.href : '';
  }

  function modelsRows(merged, search, freeOnly, state) {
    if (state === 'error') {
      return '<tr><td colspan="5" class="empty"><strong id="models-error">模型目录读取失败</strong><span>刷新后重试；失败不会被当成空目录。</span></td></tr>';
    }
    if (state === 'idle' || state === 'loading') {
      return '<tr><td colspan="5" class="empty"><strong>正在读取模型目录</strong><span>从上游拉取可用模型与倍率。</span></td></tr>';
    }
    if (!merged.length) {
      return '<tr><td colspan="5" class="empty">暂无模型目录</td></tr>';
    }
    var needle = String(search || '').trim().toLowerCase();
    var rows = merged.filter(function (row) {
      if (freeOnly && !row.free) return false;
      return !needle || (row.id + ' ' + row.display_name).toLowerCase().indexOf(needle) >= 0;
    });
    if (!rows.length) {
      return '<tr><td colspan="5" class="empty">没有匹配的模型</td></tr>';
    }
    return rows.map(function (row) {
      return '<tr><td data-label="调用 ID"><span class="code">' + escapeHTML(row.id) + '</span></td>' +
        '<td data-label="展示名"><strong>' + escapeHTML(row.display_name || row.id) + '</strong></td>' +
        '<td data-label="全球">' + rateCell(row.global) + '</td>' +
        '<td data-label="国内">' + rateCell(row.cn) + '</td>' +
        '<td data-label="操作"><button class="text-button" type="button" data-copy-id="' + escapeHTML(row.id) + '">复制 ID</button></td></tr>';
    }).join('');
  }

  function accountOptions(accounts, selected) {
    return accounts.map(function (account) {
      var label = account.label || account.name || account.auth_index;
      return '<option value="' + escapeHTML(account.auth_index) + '"' + (account.auth_index === selected ? ' selected' : '') + '>' + escapeHTML(label) + '</option>';
    }).join('');
  }

  function campaignRow(campaign) {
    var claimable = campaign.claim_status === 'CLAIMABLE' && campaign.action_type === 'CLAIM_BENEFIT';
    return '<tr><td data-label="活动">' + escapeHTML(campaign.campaign_key || campaign.campaign_id) + '</td>' +
      '<td data-label="活动 ID"><span class="code">' + escapeHTML(campaign.campaign_id) + '</span></td>' +
      '<td data-label="状态">' + escapeHTML(campaign.claim_status) + '</td>' +
      '<td data-label="权益">' + escapeHTML(campaign.benefit_kind) + ' ' + formatNumber(campaign.benefit_amount) + '</td>' +
      '<td data-label="操作">' + (claimable ? '<button class="button small" type="button" data-claim="' + escapeHTML(campaign.campaign_id) + '">领取</button>' : '-') + '</td></tr>';
  }

  function campaignsRows(campaigns) {
    if (!campaigns.length) return '<tr><td colspan="5" class="empty">暂无活动</td></tr>';
    return campaigns.map(campaignRow).join('');
  }

  window.QoderViews = {
    escapeHTML: escapeHTML,
    formatNumber: formatNumber,
    transportName: transportName,
    transportOptions: transportOptions,
    accountList: accountList,
    overview: overview,
    accountDetails: accountDetails,
    mergeModels: mergeModels,
    modelsRows: modelsRows,
    accountOptions: accountOptions,
    campaignsRows: campaignsRows
  };
}());

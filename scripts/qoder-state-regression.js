// 在已打开的 Playwright 会话运行：playwright-cli -s=<session> run-code --filename=scripts/qoder-state-regression.js
async ownerPage => {
  const context = await ownerPage.context().browser().newContext();
  const page = await context.newPage();
  const baseURL = (typeof globalThis !== 'undefined' && globalThis.__QODER_TEST_BASE) || 'http://127.0.0.1:8766/index.html';
  try {
    const failures = [];
    const check = (name, condition, detail) => {
      if (!condition) failures.push({ name, detail });
    };
    const baseAccounts = [
      {
        auth_index: 'alice-auth', name: 'qoder-alice.json', label: 'Alice', email: 'alice@example.invalid',
        transport_profile: 'cosy-api2', priority: 9, plan_tier: 'Pro', period_end: '2026-12-31T00:00:00Z',
        remaining: 300, limit: 500, used: 200, unit: 'credits', reset_at: '2026-12-01T08:00:00Z',
        quota_synced_at: '2026-11-25T00:00:00Z',
        upgrade_url: 'javascript:alert(1)',
        shared: { limit: 6000, used: 254, remaining: 5746, available: true }
      },
      {
        auth_index: 'bob-auth', name: 'qoder-bob.json', label: 'Bob', email: 'bob@example.invalid',
        transport_profile: 'cosy-api3', priority: 12,
        upgrade_url: 'https://qoder.example.invalid/upgrade'
      }
    ];
    let accounts = baseAccounts.slice();
    let models = [];
    let profileStatus = 200;
    let accountsStatus = 200;
    let modelsStatus = 200;
    let profileCalls = 0;
    let profileWrites = [];
    let deleteCalls = 0;
    let popupCount = 0;
    const holders = {};
    context.on('page', popup => { popupCount++; popup.close(); });

    const holdNext = which => {
      const holder = {};
      holder.entered = new Promise(resolve => { holder.signalEntered = resolve; });
      holder.released = new Promise(resolve => { holder.release = resolve; });
      holders[which] = holder;
      return holder;
    };
    const waitEntered = async holder => {
      let timer;
      const timeout = new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error('held response never reached the page')), 10000);
      });
      try { await Promise.race([holder.entered, timeout]); } finally { clearTimeout(timer); }
    };
    const waitAccounts = () => page.waitForFunction(() => {
      const button = document.querySelector('#refresh');
      return button && !button.disabled;
    }, null, { timeout: 10000 });
    const waitModels = () => page.waitForFunction(() => {
      const button = document.querySelector('#refresh-models');
      return button && !button.disabled;
    }, null, { timeout: 10000 });
    const waitEditRefresh = () => page.waitForFunction(() => {
      const button = document.querySelector('#edit-refresh');
      return button && !button.disabled;
    }, null, { timeout: 10000 });
    const settle = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    const hardGoto = async hash => {
      await page.goto(baseURL + hash);
      await page.reload();
    };
    const viewSelectors = { accounts: '#refresh', models: '#refresh-models', activity: '#refresh-campaigns', connection: '#key-form' };
    const view = async name => {
      await page.locator('#task-nav [data-view="' + name + '"]').click();
      await page.locator(viewSelectors[name]).waitFor();
    };
    const openAccount = async auth => {
      await page.locator('[data-edit="' + auth + '"]').click();
      await page.locator('#account-dialog[open]').waitFor();
    };
    const closeAccount = async () => {
      await page.locator('#account-dialog-cancel').click();
      await page.waitForFunction(() => !document.querySelector('#account-dialog').open);
    };

    await page.route('**/v0/management/**', async route => {
      const request = route.request();
      const url = new URL(request.url());
      const path = url.pathname;
      let which = null;
      if (path.endsWith('/qoder/accounts')) which = 'accounts';
      else if (path.endsWith('/qoder/models')) which = 'models';
      else if (path.endsWith('/qoder/accounts/activity/fetch')) which = 'activity';
      else if (path.endsWith('/qoder/accounts/profile')) { which = 'profile'; profileCalls++; profileWrites.push(JSON.parse(request.postData() || '{}')); }
      else if (path.endsWith('/qoder/accounts/quota/refresh')) which = 'quota';
      else if (path.endsWith('/qoder/accounts/campaigns/refresh')) which = 'campaigns';
      else if (path.endsWith('/qoder/accounts/campaigns/claim')) which = 'claim';
      else if (path.endsWith('/auth-files/fields')) which = 'fields';
      else if (path.endsWith('/auth-files') && request.method() === 'DELETE') which = 'delete';

      const accountsSnapshot = accounts.slice();
      const modelsSnapshot = models.slice();
      const holder = which ? holders[which] : null;
      if (holder && !holder.claimed) {
        holder.claimed = true;
        holder.signalEntered();
        await holder.released;
      }

      if (path.endsWith('/qoder/accounts')) {
        if (accountsStatus !== 200) { await route.fulfill({ status: accountsStatus, json: { error: 'synthetic accounts refusal' } }); return; }
        await route.fulfill({ json: { accounts: accountsSnapshot } });
        return;
      }
      if (path.endsWith('/qoder/models')) {
        if (modelsStatus !== 200) { await route.fulfill({ status: modelsStatus, json: { error: 'synthetic models refusal' } }); return; }
        await route.fulfill({ json: { models: modelsSnapshot } });
        return;
      }
      if (path.endsWith('/qoder/accounts/profile')) {
        await route.fulfill({ status: profileStatus, json: profileStatus === 200 ? {} : { error: 'synthetic refusal' } });
        return;
      }
      if (path.endsWith('/qoder/accounts/quota/refresh')) {
        await route.fulfill({ json: {} });
        return;
      }
      if (path.endsWith('/qoder-auth-url')) {
        await route.fulfill({ json: { url: 'javascript:alert(1)', state: 'synthetic' } });
        return;
      }
      if (path.endsWith('/get-auth-status')) {
        await route.fulfill({ json: { status: 'pending' } });
        return;
      }
      if (path.endsWith('/qoder/accounts/campaigns/refresh')) {
        await route.fulfill({ json: { campaigns: [{ campaign_id: 'synthetic-claim', campaign_key: 'Alice only', claim_status: 'CLAIMABLE', action_type: 'CLAIM_BENEFIT', benefit_kind: 'credits', benefit_amount: 10 }] } });
        return;
      }
      if (path.endsWith('/qoder/accounts/campaigns/claim')) {
        await route.fulfill({ json: { outcome: 'claimed', grant_id: 'alice-only-grant' } });
        return;
      }
      if (path.endsWith('/qoder/accounts/activity/fetch')) {
        await route.fulfill({ json: { total_credits: 801, peak_credits: 90, cumulative_days: 14, current_streak: 4, max_streak: 8, last_active_date: '2026-11-24', synced_at: '2026-11-25T00:00:00Z' } });
        return;
      }
      if (path.endsWith('/auth-files/fields')) {
        await route.fulfill({ json: {} });
        return;
      }
      if (path.endsWith('/auth-files') && request.method() === 'DELETE') {
        deleteCalls++;
        const name = url.searchParams.get('name');
        accounts = accounts.filter(account => account.name !== name);
        await route.fulfill({ json: {} });
        return;
      }
      await route.fulfill({ json: {} });
    });

    await page.goto(baseURL);
    await page.evaluate(() => localStorage.setItem('qoder-cpa-mgmt-key', 'synthetic-only'));
    const bootHolder = holdNext('accounts');
    await page.evaluate(() => { history.replaceState(null, '', location.pathname); });
    await page.reload();
    await waitEntered(bootHolder);
    const loadingOverview = await page.evaluate(() => ({
      total: document.querySelector('#overview-total').textContent,
      sync: document.querySelector('#overview-sync').textContent
    }));
    bootHolder.release();
    await waitAccounts();
    const readyOverview = await page.evaluate(() => ({
      total: document.querySelector('#overview-total').textContent,
      attention: document.querySelector('#overview-attention').textContent,
      rows: document.querySelectorAll('[data-auth-row]').length
    }));
    check('overview shows loading then counts', loadingOverview.total === '-' && loadingOverview.sync === '正在读取账号' && readyOverview.total === '2' && readyOverview.attention === '0' && readyOverview.rows === 2, { loadingOverview, readyOverview });

    try {
      await openAccount('alice-auth');
      await page.locator('#edit-transport').selectOption('bearer-openai');
      await page.locator('#edit-priority').fill('27');
      await page.locator('#edit-refresh').click();
      await page.waitForFunction(() => document.querySelector('#account-edit-status').textContent === '配额已刷新。');
      const draft = await page.evaluate(() => ({
        transport: document.querySelector('#edit-transport').value,
        priority: document.querySelector('#edit-priority').value,
        open: document.querySelector('#account-dialog').open
      }));
      check('editor draft survives quota refresh', draft.transport === 'bearer-openai' && draft.priority === '27' && draft.open, draft);
    } catch (error) { failures.push({ name: 'editor draft survives quota refresh', detail: String(error) }); }

    try {
      const writesBefore = profileCalls;
      await closeAccount();
      await openAccount('alice-auth');
      const reopened = await page.evaluate(() => ({
        transport: document.querySelector('#edit-transport').value,
        priority: document.querySelector('#edit-priority').value
      }));
      check('editor cancel discards unsaved draft without writing', reopened.transport === 'cosy-api2' && reopened.priority === '9' && profileCalls === writesBefore, { reopened, profileCalls });
      await closeAccount();
    } catch (error) { failures.push({ name: 'editor cancel discards unsaved draft without writing', detail: String(error) }); }

    try {
      await view('activity');
      await page.locator('#g1-auth').selectOption('bob-auth');
      await view('accounts');
      await page.locator('#refresh').click();
      await waitAccounts();
      await view('activity');
      const selected = await page.locator('#g1-auth').inputValue();
      check('account refresh keeps activity selection', selected === 'bob-auth', { selected });
    } catch (error) { failures.push({ name: 'account refresh keeps activity selection', detail: String(error) }); }

    try {
      await view('accounts');
      await openAccount('alice-auth');
      await page.locator('#edit-priority').fill('27');
      await page.locator('#edit-priority').focus();
      const holder = holdNext('accounts');
      await page.locator('#edit-refresh').click();
      await waitEntered(holder);
      await page.locator('#edit-priority').fill('28');
      holder.release();
      await waitEditRefresh();
      const restored = await page.evaluate(() => ({
        active: document.activeElement.id,
        priority: document.querySelector('#edit-priority').value,
        transport: document.querySelector('#edit-transport').value
      }));
      check('held account read restores focused priority editor', restored.active === 'edit-priority' && restored.priority === '28' && restored.transport === 'cosy-api2', restored);
    } catch (error) { failures.push({ name: 'held account read restores focused priority editor', detail: String(error) }); }

    try {
      await page.locator('#edit-priority').focus();
      const holder = holdNext('accounts');
      await page.locator('#edit-refresh').click();
      await waitEntered(holder);
      await page.locator('#edit-transport').focus();
      holder.release();
      await waitEditRefresh();
      const active = await page.evaluate(() => document.activeElement.id);
      check('held account read does not steal focus after the user moves away', active === 'edit-transport', { active });
      await closeAccount();
    } catch (error) { failures.push({ name: 'held account read does not steal focus after the user moves away', detail: String(error) }); }

    for (const which of ['campaigns', 'activity', 'claim']) {
      try {
        await view('activity');
        await page.locator('#g1-auth').selectOption('alice-auth');
        if (which === 'claim') {
          await page.locator('#refresh-campaigns').click();
          await page.locator('[data-claim="synthetic-claim"]').waitFor();
        }
        const holder = holdNext(which);
        const button = which === 'campaigns' ? '#refresh-campaigns' : which === 'activity' ? '#fetch-activity' : '[data-claim="synthetic-claim"]';
        const pathSuffix = which === 'campaigns' ? '/campaigns/refresh' : which === 'activity' ? '/activity/fetch' : '/campaigns/claim';
        await page.locator(button).click();
        await waitEntered(holder);
        await page.locator('#g1-auth').selectOption('bob-auth');
        const finished = page.waitForEvent('requestfinished', request => request.url().endsWith(pathSuffix));
        holder.release();
        await finished;
        await settle();
        const displayed = await page.locator('#campaigns, #g1-credits, #g1-claim').allTextContents();
        check(`${which} result stays with its original account`, JSON.stringify(displayed) === JSON.stringify(['尚未刷新', '尚未查询', '等待操作']), displayed);
      } catch (error) { failures.push({ name: `${which} result stays with its original account`, detail: String(error) }); }
    }

    try {
      await view('activity');
      await page.locator('#g1-auth').selectOption('alice-auth');
      await page.locator('#fetch-activity').click();
      await page.waitForFunction(() => document.querySelector('#g1-credits').textContent !== '尚未查询');
      const holder = holdNext('activity');
      await page.locator('#fetch-activity').click();
      await waitEntered(holder);
      await view('accounts');
      await openAccount('alice-auth');
      await page.locator('#edit-delete').click();
      await page.locator('#delete-dialog[open]').waitFor();
      const finished = page.waitForEvent('requestfinished', request => request.url().endsWith('/activity/fetch'));
      await page.locator('#delete-confirm').click();
      await page.waitForFunction(() => !document.querySelector('[data-edit="alice-auth"]'));
      holder.release();
      await finished;
      await settle();
      await view('activity');
      const stale = await page.evaluate(() => ({
        credits: document.querySelector('#g1-credits').textContent,
        seat: document.querySelector('#g1-seat').textContent,
        claim: document.querySelector('#g1-claim').textContent,
        selected: document.querySelector('#g1-auth').value
      }));
      check('account removal invalidates the in-flight activity result', stale.credits === '尚未查询' && stale.seat === '尚未查询' && stale.claim === '等待操作' && stale.selected === 'bob-auth', stale);
    } catch (error) { failures.push({ name: 'account removal invalidates the in-flight activity result', detail: String(error) }); }

    try {
      await view('accounts');
      await openAccount('bob-auth');
      profileStatus = 503;
      const holder = holdNext('profile');
      const save = page.locator('#save-transport');
      await save.click();
      await waitEntered(holder);
      const box = await save.boundingBox();
      if (!box) throw new Error('save button has no hit target');
      await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
      check('pending save rejects a second press', profileCalls === 1 && await save.isDisabled(), { profileCalls });
      holder.release();
      await page.waitForFunction(() => document.querySelector('#account-edit-status').textContent.includes('传输方式保存失败'));
      const failedDraft = await page.evaluate(() => ({
        transport: document.querySelector('#edit-transport').value,
        status: document.querySelector('#account-edit-status').textContent
      }));
      check('failed save keeps the unsaved draft', failedDraft.transport === 'cosy-api3' && failedDraft.status.includes('草稿已保留'), failedDraft);
      profileStatus = 200;
      await save.click();
      await page.waitForFunction(() => document.querySelector('#account-edit-status').textContent === '传输方式已保存。');
      check('failed save can retry', profileCalls === 2 && await save.isEnabled() && await page.locator('#status').textContent() === '传输方式已保存。', { profileCalls });
      await closeAccount();
    } catch (error) { failures.push({ name: 'profile save twice and retry', detail: String(error) }); }

    try {
      accounts = baseAccounts.slice();
      await page.locator('#refresh').click();
      await waitAccounts();
      await openAccount('alice-auth');
      await page.locator('#edit-transport').selectOption('bearer-openai');
      const writesBefore = profileWrites.length;
      await page.locator('#save-transport').click();
      await page.waitForFunction(() => document.querySelector('#account-edit-status').textContent === '传输方式已保存。');
      const saved = await page.evaluate(() => document.querySelector('[data-auth-row="alice-auth"]').textContent);
      check('saved transport redraws the visible account row', saved.includes('国际 Bearer') && profileWrites.length === writesBefore + 1 && profileWrites[writesBefore].auth_index === 'alice-auth' && profileWrites[writesBefore].transport_profile === 'bearer-openai', { saved, profileWrites: profileWrites.slice(writesBefore) });
      await closeAccount();
      await openAccount('alice-auth');
      const reopened = await page.locator('#edit-transport').inputValue();
      await page.locator('#edit-transport').selectOption('cosy-cn');
      await closeAccount();
      await openAccount('alice-auth');
      const afterCancel = await page.locator('#edit-transport').inputValue();
      check('cancel keeps the persisted transport on close and reopen', reopened === 'bearer-openai' && afterCancel === 'bearer-openai', { reopened, afterCancel });
      await closeAccount();
    } catch (error) { failures.push({ name: 'saved transport redraws and cancel preserves persisted value', detail: String(error) }); }

    try {
      accounts = baseAccounts.slice();
      await page.locator('#refresh').click();
      await waitAccounts();
      await openAccount('alice-auth');
      await page.locator('#edit-transport').selectOption('bearer-openai');
      const writesBefore = profileWrites.length;
      const holder = holdNext('profile');
      await page.locator('#save-transport').click();
      await waitEntered(holder);
      await closeAccount();
      await openAccount('alice-auth');
      const beforeRelease = await page.locator('#edit-transport').inputValue();
      await page.locator('#edit-transport').selectOption('cosy-cn');
      const finished = page.waitForEvent('requestfinished', request => request.url().endsWith('/qoder/accounts/profile'));
      holder.release();
      await finished;
      await settle();
      const afterRelease = await page.evaluate(() => ({
        row: document.querySelector('[data-auth-row="alice-auth"]').textContent,
        transport: document.querySelector('#edit-transport').value,
        status: document.querySelector('#account-edit-status').textContent
      }));
      const write = profileWrites[writesBefore] || {};
      await closeAccount();
      await openAccount('alice-auth');
      const persistedValue = await page.locator('#edit-transport').inputValue();
      check('late save after close and reopen updates the account but not the newer editor', beforeRelease === 'cosy-api2' && write.transport_profile === 'bearer-openai' && afterRelease.row.includes('国际 Bearer') && afterRelease.transport === 'cosy-cn' && afterRelease.status !== '传输方式已保存。' && persistedValue === 'bearer-openai', { beforeRelease, write, afterRelease, persistedValue });
      await closeAccount();
    } catch (error) { failures.push({ name: 'late save after close and reopen updates the account but not the newer editor', detail: String(error) }); }

    try {
      await page.locator('#login').click();
      await page.locator('#login-dialog[open]').waitFor();
      await page.locator('#login-start').click();
      await page.waitForFunction(() => document.querySelector('#login-status').textContent.includes('协议不受支持'));
      check('unsafe OAuth URL never opens', popupCount === 0, { popupCount });
      await page.locator('#login-dialog-close').click();
      await page.waitForFunction(() => !document.querySelector('#login-dialog').open);
    } catch (error) { failures.push({ name: 'unsafe OAuth URL never opens', detail: String(error) }); }

    accounts = baseAccounts.slice();
    accountsStatus = 200;
    modelsStatus = 200;
    await page.locator('#refresh').click();
    await waitAccounts();
    const deletesBeforeDialog = deleteCalls;

    try {
      await openAccount('alice-auth');
      await page.locator('#edit-delete').click();
      await page.locator('#delete-dialog[open]').waitFor();
      const opened = await page.evaluate(() => ({ open: document.querySelector('#delete-dialog').open, focused: document.activeElement.id }));
      await page.locator('#delete-cancel').click();
      const stillOpen = await page.evaluate(() => document.querySelector('#delete-dialog').open);
      check('delete dialog cancel closes without deleting', opened.open && opened.focused === 'delete-cancel' && !stillOpen && deleteCalls === deletesBeforeDialog, { opened, stillOpen, deleteCalls });
    } catch (error) { failures.push({ name: 'delete dialog cancel closes without deleting', detail: String(error) }); }

    try {
      await page.locator('#edit-delete').click();
      await page.locator('#delete-dialog[open]').waitFor();
      const opened = await page.evaluate(() => document.querySelector('#delete-dialog').open);
      await page.keyboard.press('Escape');
      const stillOpen = await page.evaluate(() => document.querySelector('#delete-dialog').open);
      check('delete dialog escape closes without deleting', opened && !stillOpen && deleteCalls === deletesBeforeDialog, { opened, stillOpen, deleteCalls });
      await closeAccount();
    } catch (error) { failures.push({ name: 'delete dialog escape closes without deleting', detail: String(error) }); }

    try {
      accountsStatus = 503;
      await page.locator('#refresh').click();
      await waitAccounts();
      const failed = await page.evaluate(() => ({
        attention: document.querySelector('#overview-attention').textContent,
        row: document.querySelector('#account-list').textContent
      }));
      accountsStatus = 200;
      await page.locator('#refresh').click();
      await waitAccounts();
      const recovered = await page.evaluate(() => ({
        total: document.querySelector('#overview-total').textContent,
        rows: document.querySelectorAll('[data-auth-row]').length
      }));
      check('overview reports failure and recovers on retry', failed.attention === '读取失败' && failed.row.includes('synthetic accounts refusal') && recovered.total === '2' && recovered.rows === 2, { failed, recovered });
    } catch (error) { failures.push({ name: 'overview reports failure and recovers on retry', detail: String(error) }); }

    try {
      await view('models');
      await waitModels();
      models = [];
      const holder = holdNext('models');
      await page.locator('#refresh-models').click();
      await waitEntered(holder);
      const loading = await page.locator('#models-status').textContent();
      holder.release();
      await waitModels();
      const empty = await page.evaluate(() => ({
        count: document.querySelector('#models-count').textContent,
        body: document.querySelector('#models-body').textContent
      }));
      modelsStatus = 503;
      await page.locator('#refresh-models').click();
      await waitModels();
      const failed = await page.locator('#models-status').textContent();
      modelsStatus = 200;
      models = [{ id: 'm1', display_name: 'Model One', profile: 'cosy-api2', rate: 0.5, is_free: false }];
      await page.locator('#refresh-models').click();
      await waitModels();
      const recovered = await page.evaluate(() => ({
        count: document.querySelector('#models-count').textContent,
        body: document.querySelector('#models-body').textContent
      }));
      check('models shows loading, empty, failure and recovers', loading === '正在读取模型目录' && empty.count === '0' && empty.body.includes('暂无模型目录') && failed === '刷新失败' && recovered.count === '1' && recovered.body.includes('m1'), { loading, empty, failed, recovered });
    } catch (error) { failures.push({ name: 'models shows loading, empty, failure and recovers', detail: String(error) }); }

    try {
      await view('accounts');
      accounts = [];
      await page.locator('#refresh').click();
      await waitAccounts();
      const empty = await page.evaluate(() => ({
        total: document.querySelector('#overview-total').textContent,
        row: document.querySelector('#account-list').textContent
      }));
      await view('activity');
      const activitySelect = await page.locator('#g1-auth').count();
      check('overview shows zero and empty list for no accounts', empty.total === '0' && empty.row.includes('还没有 Qoder 账号') && activitySelect === 0, { empty, activitySelect });
    } catch (error) { failures.push({ name: 'overview shows zero and empty list for no accounts', detail: String(error) }); }

    try {
      await view('accounts');
      accounts = baseAccounts.slice();
      await page.locator('#refresh').click();
      await waitAccounts();
      const quota = await page.evaluate(() => ({
        alice: document.querySelector('[data-auth-row="alice-auth"]').textContent,
        bob: document.querySelector('[data-auth-row="bob-auth"]').textContent
      }));
      check('unknown quota is shown as unread, never 0 / 0', quota.bob.includes('未读取') && !quota.bob.includes('0 / 0') && quota.alice.includes('300 / 500'), quota);
    } catch (error) { failures.push({ name: 'unknown quota is shown as unread, never 0 / 0', detail: String(error) }); }

    try {
      const holder = holdNext('accounts');
      await page.locator('#refresh').click();
      await waitEntered(holder);
      await view('connection');
      accounts = [{ auth_index: 'carol-auth', name: 'qoder-carol.json', label: 'Carol', email: 'carol@example.invalid', transport_profile: 'cosy-api2', priority: 1 }];
      await page.locator('#mgmt-key').fill('synthetic-second');
      await page.locator('#save-key').click();
      await page.locator('[data-edit="carol-auth"]').waitFor();
      const finished = page.waitForEvent('requestfinished', request => request.url().endsWith('/qoder/accounts'));
      holder.release();
      await finished;
      await settle();
      const after = await page.evaluate(() => ({
        total: document.querySelector('#overview-total').textContent,
        alice: document.querySelectorAll('[data-auth-row="alice-auth"]').length,
        carol: document.querySelectorAll('[data-auth-row="carol-auth"]').length,
        dialog: document.querySelector('#account-dialog').open
      }));
      check('key switch drops the stale accounts response', after.total === '1' && after.alice === 0 && after.carol === 1 && !after.dialog, after);
    } catch (error) { failures.push({ name: 'key switch drops the stale accounts response', detail: String(error) }); }

    await page.evaluate(() => localStorage.setItem('qoder-cpa-mgmt-key', 'synthetic-only'));
    accounts = baseAccounts.slice();
    models = [{ id: 'm1', display_name: 'Model One', profile: 'cosy-api2', rate: 0.5, is_free: false }];
    accountsStatus = 200;
    modelsStatus = 200;

    try {
      await hardGoto('#models');
      await waitModels();
      const booted = await page.evaluate(() => ({
        hash: location.hash,
        active: document.querySelector('#task-nav .is-active').dataset.view,
        count: document.querySelector('#models-count').textContent,
        status: document.querySelector('#models-status').textContent,
        accountsButton: Boolean(document.querySelector('#refresh'))
      }));
      check('#models boot renders the models view', booted.hash === '#models' && booted.active === 'models' && booted.count === '1' && booted.status.indexOf('更新于') === 0 && !booted.accountsButton, booted);
    } catch (error) { failures.push({ name: '#models boot renders the models view', detail: String(error) }); }

    try {
      await hardGoto('#activity');
      await page.locator('#g1-auth').waitFor();
      const booted = await page.evaluate(() => ({
        hash: location.hash,
        active: document.querySelector('#task-nav .is-active').dataset.view,
        options: document.querySelectorAll('#g1-auth option').length,
        selected: document.querySelector('#g1-auth').value
      }));
      check('#activity boot renders the activity view with accounts', booted.hash === '#activity' && booted.active === 'activity' && booted.options === 2 && booted.selected === 'alice-auth', booted);
    } catch (error) { failures.push({ name: '#activity boot renders the activity view with accounts', detail: String(error) }); }

    try {
      await page.evaluate(() => localStorage.removeItem('qoder-cpa-mgmt-key'));
      await hardGoto('#models');
      await page.locator('#key-form').waitFor();
      const booted = await page.evaluate(() => ({
        hash: location.hash,
        active: document.querySelector('#task-nav .is-active').dataset.view,
        status: document.querySelector('#status').textContent,
        modelsButton: Boolean(document.querySelector('#refresh-models'))
      }));
      check('boot without a key shows connection and rewrites the hash', booted.hash === '#connection' && booted.active === 'connection' && booted.status.includes('未检测到管理密钥') && !booted.modelsButton, booted);
      await page.evaluate(() => localStorage.setItem('qoder-cpa-mgmt-key', 'synthetic-only'));
      await hardGoto('');
      await waitAccounts();
    } catch (error) { failures.push({ name: 'boot without a key shows connection and rewrites the hash', detail: String(error) }); }

    try {
      await view('models');
      await waitModels();
      const holder = holdNext('models');
      await page.locator('#refresh-models').click();
      await waitEntered(holder);
      const loading = await page.evaluate(() => ({
        count: document.querySelector('#models-count-label').textContent,
        rows: document.querySelector('#models-body').textContent
      }));
      modelsStatus = 503;
      holder.release();
      await waitModels();
      const failed = await page.evaluate(() => ({
        count: document.querySelector('#models-count-label').textContent,
        status: document.querySelector('#models-status').textContent,
        rows: document.querySelector('#models-body').textContent
      }));
      modelsStatus = 200;
      models = [];
      await page.locator('#refresh-models').click();
      await waitModels();
      const empty = await page.evaluate(() => ({
        count: document.querySelector('#models-count').textContent,
        rows: document.querySelector('#models-body').textContent
      }));
      check('models loading and failure never read as zero or empty', loading.count === '正在读取' && !loading.rows.includes('暂无模型目录') && failed.count === '未读取' && failed.status === '刷新失败' && failed.rows.includes('模型目录读取失败') && !failed.rows.includes('暂无模型目录') && empty.count === '0' && empty.rows.includes('暂无模型目录'), { loading, failed, empty });
    } catch (error) { failures.push({ name: 'models loading and failure never read as zero or empty', detail: String(error) }); }

    try {
      models = [{ id: 'm1', display_name: 'Model One', profile: 'cosy-api2', rate: 0.5, is_free: false }];
      await view('accounts');
      await openAccount('alice-auth');
      const upgrade = await page.evaluate(() => {
        const rows = Array.from(document.querySelectorAll('#account-details .detail-list div'));
        const row = rows.find(item => item.querySelector('dt').textContent === '升级链接');
        return {
          text: row ? row.querySelector('dd').textContent : '',
          anchors: document.querySelectorAll('#account-details a[href^="javascript"]').length
        };
      });
      await closeAccount();
      await openAccount('bob-auth');
      const safe = await page.evaluate(() => {
        const rows = Array.from(document.querySelectorAll('#account-details .detail-list div'));
        const row = rows.find(item => item.querySelector('dt').textContent === '升级链接');
        const link = row ? row.querySelector('dd a') : null;
        return { text: row ? row.querySelector('dd').textContent : '', href: link ? link.getAttribute('href') : '', rel: link ? link.getAttribute('rel') : '' };
      });
      await closeAccount();
      check('upgrade link only renders http(s) targets', upgrade.text === '未提供' && upgrade.anchors === 0 && safe.href === 'https://qoder.example.invalid/upgrade' && safe.rel.includes('noopener'), { upgrade, safe });
    } catch (error) { failures.push({ name: 'upgrade link only renders http(s) targets', detail: String(error) }); }

    try {
      await openAccount('alice-auth');
      await page.locator('#edit-priority').fill('27');
      const holder = holdNext('fields');
      await page.locator('#save-priority').click();
      await waitEntered(holder);
      await page.locator('#edit-priority').fill('28');
      holder.release();
      await page.waitForFunction(() => document.querySelector('#account-edit-status').textContent.includes('优先级已保存'));
      const late = await page.evaluate(() => ({
        input: document.querySelector('#edit-priority').value
      }));
      await closeAccount();
      await openAccount('alice-auth');
      const reopened = await page.evaluate(() => document.querySelector('#edit-priority').value);
      check('late priority save does not clobber a newer draft', late.input === '28' && reopened === '27', { late, reopened });
      await closeAccount();
    } catch (error) { failures.push({ name: 'late priority save does not clobber a newer draft', detail: String(error) }); }

    try {
      await openAccount('alice-auth');
      accounts = baseAccounts.filter(account => account.auth_index !== 'alice-auth');
      const holder = holdNext('accounts');
      await page.locator('#edit-refresh').click();
      await waitEntered(holder);
      holder.release();
      await page.waitForFunction(() => !document.querySelector('#account-dialog').open);
      const closed = await page.evaluate(() => ({
        open: document.querySelector('#account-dialog').open,
        status: document.querySelector('#status').textContent,
        rows: document.querySelectorAll('[data-auth-row]').length
      }));
      check('account refresh closes an editor whose account vanished', !closed.open && closed.status.includes('编辑器已关闭') && closed.rows === 1, closed);
    } catch (error) { failures.push({ name: 'account refresh closes an editor whose account vanished', detail: String(error) }); }

    try {
      accounts = baseAccounts.slice();
      await page.locator('#refresh').click();
      await waitAccounts();
      await openAccount('alice-auth');
      await page.locator('#edit-transport').selectOption('bearer-openai');
      const holder = holdNext('profile');
      await page.locator('#save-transport').click();
      await waitEntered(holder);
      accounts = baseAccounts.map(account => account.auth_index === 'alice-auth' ? Object.assign({}, account, { transport_profile: 'cosy-cn' }) : account);
      const accountsHolder = holdNext('accounts');
      await page.locator('#edit-refresh').click();
      await waitEntered(accountsHolder);
      accountsHolder.release();
      await waitAccounts();
      holder.release();
      await page.waitForFunction(() => document.querySelector('#account-edit-status').textContent === '传输方式已保存。');
      const refreshed = await page.evaluate(() => document.querySelector('[data-auth-row="alice-auth"]').textContent);
      const select = await page.locator('#edit-transport').inputValue();
      check('stale save does not overwrite a refreshed list value', refreshed.includes('国内站') && select === 'bearer-openai', { refreshed, select });
      await closeAccount();
    } catch (error) { failures.push({ name: 'stale save does not overwrite a refreshed list value', detail: String(error) }); }

    if (failures.length) throw new Error('state retention regression failed: ' + JSON.stringify(failures));
    return '29 browser cases passed: boot loading, drafts, cancel, activity selection, focus, three stale-response paths, account removal, duplicate save, refusal retry, unsafe OAuth, delete dialog cancel/escape, overview failure/retry, models loading/empty/failure, empty accounts, unknown quota, key switch, hash boot models/activity, keyless boot, models truthfulness, upgrade link, late priority save, vanished editor, stale save, saved transport repaint, late save after close and reopen';
  } finally {
    await context.close();
  }
}

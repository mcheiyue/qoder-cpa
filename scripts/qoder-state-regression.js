// 在已打开的 Playwright 会话运行：playwright-cli -s=<session> run-code --filename=scripts/qoder-state-regression.js
async ownerPage => {
  const context = await ownerPage.context().browser().newContext();
  const page = await context.newPage();
  try {
  const failures = [];
  const check = (name, condition, detail) => {
    if (!condition) failures.push({ name, detail });
  };
  const baseAccounts = [
    { auth_index: 'alice-auth', name: 'alice', label: 'Alice', email: 'alice@example.invalid', transport_profile: 'cosy-api2', priority: 9 },
    { auth_index: 'bob-auth', name: 'bob', label: 'Bob', email: 'bob@example.invalid', transport_profile: 'cosy-api3', priority: 12 },
  ];
  let accounts = baseAccounts.slice();
  const holders = { accounts: null, models: null, activity: null, profile: null, campaigns: null, claim: null };
  let profileCalls = 0;
  let profileStatus = 200;
  let accountsStatus = 200;
  let modelsStatus = 200;
  let deleteCalls = 0;
  let popupCount = 0;
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
  const waitSettled = () => page.waitForFunction(() => !document.querySelector('#refresh').disabled, null, { timeout: 10000 });
  const waitModelsSettled = () => page.waitForFunction(() => !document.querySelector('#refresh-models').disabled, null, { timeout: 10000 });

  await page.route('**/v0/management/**', async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let which = null;
    if (path.endsWith('/qoder/accounts')) which = 'accounts';
    else if (path.endsWith('/qoder/models')) which = 'models';
    else if (path.endsWith('/qoder/accounts/activity/fetch')) which = 'activity';
    else if (path.endsWith('/qoder/accounts/profile')) { which = 'profile'; profileCalls++; }
    else if (path.endsWith('/qoder/accounts/campaigns/refresh')) which = 'campaigns';
    else if (path.endsWith('/qoder/accounts/campaigns/claim')) which = 'claim';
    const holder = which ? holders[which] : null;
    if (holder && !holder.claimed) {
      holder.claimed = true;
      holder.signalEntered();
      await holder.released;
    }
    if (path.endsWith('/qoder/accounts')) {
      if (accountsStatus !== 200) { await route.fulfill({ status: accountsStatus, json: { error: 'synthetic accounts refusal' } }); return; }
      await route.fulfill({ json: { accounts } }); return;
    }
    if (path.endsWith('/qoder/models')) {
      if (modelsStatus !== 200) { await route.fulfill({ status: modelsStatus, json: { error: 'synthetic models refusal' } }); return; }
      await route.fulfill({ json: { models: [] } }); return;
    }
    if (path.endsWith('/qoder/accounts/profile')) {
      await route.fulfill({ status: profileStatus, json: profileStatus === 200 ? {} : { error: 'synthetic refusal' } });
      return;
    }
    if (path.endsWith('/qoder-auth-url')) {
      await route.fulfill({ json: { url: 'javascript:alert(1)', state: 'synthetic' } });
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
    if (request.method() === 'DELETE') {
      deleteCalls++;
      const name = new URL(request.url()).searchParams.get('name');
      accounts = accounts.filter(account => account.name !== name);
      await route.fulfill({ json: {} });
      return;
    }
    await route.fulfill({ json: {} });
  });

  const alice = () => page.locator('#accounts tr').filter({ hasText: 'Alice' });
  const readRow = () => alice().evaluate(row => ({
    profile: row.querySelector('select[data-auth]').value,
    priority: row.querySelector('input[data-pri]').value,
  }));

  await page.goto('http://127.0.0.1:8766/index.html');
  await page.evaluate(() => localStorage.setItem('qoder-cpa-mgmt-key', 'synthetic-only'));
  await page.reload();
  await alice().locator('select[data-auth]').waitFor();

  try {
    await page.locator('#g1-auth').selectOption('bob-auth');
    await alice().locator('select[data-auth]').selectOption('bearer-openai');
    await alice().locator('input[data-pri]').fill('27');
    await page.locator('#refresh').click();
    await waitSettled();
    const row = await readRow();
    const activity = await page.locator('#g1-auth').inputValue();
    check('refresh keeps edited profile/priority/activity', row.profile === 'bearer-openai' && row.priority === '27' && activity === 'bob-auth', { row, activity });
  } catch (error) { failures.push({ name: 'refresh keeps edited profile/priority/activity', detail: String(error) }); }

  try {
    await alice().locator('select[data-auth]').selectOption('bearer-openai');
    await alice().locator('input[data-pri]').fill('27');
    const holder = holdNext('accounts');
    await page.locator('#refresh').click();
    await waitEntered(holder);
    await alice().locator('input[data-pri]').fill('28');
    holder.release();
    await waitSettled();
    const restored = await page.evaluate(() => {
      const row = Array.from(document.querySelectorAll('#accounts tr')).find(tr => tr.textContent.includes('Alice'));
      return {
        active: document.activeElement.getAttribute('aria-label') || document.activeElement.id,
        priority: row.querySelector('input[data-pri]').value,
        profile: row.querySelector('select[data-auth]').value,
      };
    });
    check('held account read restores focused priority editor', restored.active === 'Alice 优先级' && restored.priority === '28' && restored.profile === 'bearer-openai', restored);
  } catch (error) { failures.push({ name: 'held account read restores focused priority editor', detail: String(error) }); }

  try {
    await alice().locator('input[data-pri]').focus();
    const holder = holdNext('accounts');
    await page.locator('#refresh').click();
    await waitEntered(holder);
    await page.locator('#g1-auth').focus();
    holder.release();
    await waitSettled();
    const active = await page.evaluate(() => document.activeElement.id);
    check('held account read does not steal focus after the user moves away', active === 'g1-auth', { active });
  } catch (error) { failures.push({ name: 'held account read does not steal focus after the user moves away', detail: String(error) }); }

  for (const which of ['campaigns', 'activity', 'claim']) {
    try {
      await page.locator('#g1-auth').selectOption('alice-auth');
      if (which === 'claim') {
        await page.locator('#refresh-campaigns').click();
        await page.locator('[data-claim="synthetic-claim"]').waitFor();
      }
      const holder = holdNext(which);
      const button = which === 'campaigns' ? '#refresh-campaigns' : which === 'activity' ? '#fetch-activity' : '[data-claim="synthetic-claim"]';
      const path = which === 'campaigns' ? '/campaigns/refresh' : which === 'activity' ? '/activity/fetch' : '/campaigns/claim';
      await page.locator(button).click();
      await waitEntered(holder);
      await page.locator('#g1-auth').selectOption('bob-auth');
      const finished = page.waitForEvent('requestfinished', request => request.url().endsWith(path));
      holder.release();
      await finished;
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
      const displayed = await page.locator('#campaigns, #g1-credits, #g1-claim').allTextContents();
      check(`${which} result stays with its original account`, JSON.stringify(displayed) === JSON.stringify(['尚未刷新', '尚未查询', '等待操作']), displayed);
    } catch (error) { failures.push({ name: `${which} result stays with its original account`, detail: String(error) }); }
  }

  try {
    await page.locator('#g1-auth').selectOption('alice-auth');
    await page.locator('#fetch-activity').click();
    await page.waitForFunction(() => !document.querySelector('#fetch-activity').disabled && document.querySelector('#g1-credits').textContent !== '尚未查询', null, { timeout: 10000 });
    const holder = holdNext('activity');
    await page.locator('#fetch-activity').click();
    await waitEntered(holder);
    await alice().getByRole('button', { name: '删除', exact: true }).click();
    await page.locator('#delete-confirm').click();
    await waitSettled();
    const finished = page.waitForEvent('requestfinished', request => request.url().endsWith('/activity/fetch'));
    holder.release();
    await finished;
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    const stale = await page.evaluate(() => ({
      credits: document.querySelector('#g1-credits').textContent,
      seat: document.querySelector('#g1-seat').textContent,
      summary: document.querySelector('#summary-activity .summary-value').textContent,
      selected: document.querySelector('#g1-auth').value,
    }));
    check('account removal invalidates the in-flight activity result', stale.credits === '尚未查询' && stale.seat === '尚未查询' && stale.summary === '待查询' && stale.selected === 'bob-auth', stale);
  } catch (error) { failures.push({ name: 'account removal invalidates the in-flight activity result', detail: String(error) }); }

  try {
    const save = page.locator('[data-save="bob-auth"]');
    profileStatus = 503;
    const holder = holdNext('profile');
    await save.click();
    await waitEntered(holder);
    const box = await save.boundingBox();
    if (!box) throw new Error('save button has no hit target');
    await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
    check('pending save rejects a second press', profileCalls === 1 && await save.isDisabled(), { profileCalls });
    holder.release();
    await page.waitForFunction(() => document.querySelector('#status').textContent === 'synthetic refusal');
    profileStatus = 200;
    await save.click();
    await page.waitForFunction(() => document.querySelector('#status').textContent === '传输方式已保存。');
    check('failed save can retry', profileCalls === 2 && await save.isEnabled(), { profileCalls });
  } catch (error) { failures.push({ name: 'profile save twice and retry', detail: String(error) }); }

  try {
    await page.locator('#login').click();
    await page.waitForFunction(() => document.querySelector('#status').textContent.includes('协议不受支持'));
    check('unsafe OAuth URL never opens', popupCount === 0, { popupCount });
  } catch (error) { failures.push({ name: 'unsafe OAuth URL never opens', detail: String(error) }); }

  accounts = baseAccounts.slice();
  accountsStatus = 200;
  modelsStatus = 200;
  const deletesBeforeDialog = deleteCalls;
  await page.locator('#refresh').click();
  await waitSettled();

  try {
    await alice().getByRole('button', { name: '删除', exact: true }).click();
    const opened = await page.evaluate(() => ({ open: document.querySelector('#delete-dialog').open, focused: document.activeElement.id }));
    await page.locator('#delete-cancel').click();
    const stillOpen = await page.evaluate(() => document.querySelector('#delete-dialog').open);
    check('delete dialog cancel closes without deleting', opened.open && opened.focused === 'delete-cancel' && !stillOpen && deleteCalls === deletesBeforeDialog, { opened, stillOpen, deleteCalls });
  } catch (error) { failures.push({ name: 'delete dialog cancel closes without deleting', detail: String(error) }); }

  try {
    await alice().getByRole('button', { name: '删除', exact: true }).click();
    const opened = await page.evaluate(() => document.querySelector('#delete-dialog').open);
    await page.keyboard.press('Escape');
    const stillOpen = await page.evaluate(() => document.querySelector('#delete-dialog').open);
    check('delete dialog escape closes without deleting', opened && !stillOpen && deleteCalls === deletesBeforeDialog, { opened, stillOpen, deleteCalls });
  } catch (error) { failures.push({ name: 'delete dialog escape closes without deleting', detail: String(error) }); }

  try {
    const holder = holdNext('accounts');
    await page.locator('#refresh').click();
    await waitEntered(holder);
    const loading = await page.evaluate(() => ({
      value: document.querySelector('#summary-accounts .summary-value').textContent,
      detail: document.querySelector('#summary-accounts .summary-detail').textContent,
    }));
    holder.release();
    await waitSettled();
    const ready = await page.evaluate(() => ({
      value: document.querySelector('#summary-accounts .summary-value').textContent,
      detail: document.querySelector('#summary-accounts .summary-detail').textContent,
    }));
    check('summary accounts shows loading then counts', loading.value === '未知' && loading.detail === '正在读取账号' && ready.value === '2' && ready.detail === '正常 0 · 耗尽 0 · 需重新授权 0', { loading, ready });
  } catch (error) { failures.push({ name: 'summary accounts shows loading then counts', detail: String(error) }); }

  try {
    accountsStatus = 503;
    await page.locator('#refresh').click();
    await waitSettled();
    const failed = await page.evaluate(() => ({
      value: document.querySelector('#summary-accounts .summary-value').textContent,
      detail: document.querySelector('#summary-accounts .summary-detail').textContent,
      row: document.querySelector('#accounts').textContent,
    }));
    accountsStatus = 200;
    await page.locator('#refresh').click();
    await waitSettled();
    const recovered = await page.evaluate(() => ({
      value: document.querySelector('#summary-accounts .summary-value').textContent,
      rows: document.querySelectorAll('#accounts tr').length,
    }));
    check('summary accounts reports failure and recovers on retry', failed.value === '读取失败' && failed.detail === '账号读取失败，请刷新重试' && failed.row.includes('synthetic accounts refusal') && recovered.value === '2' && recovered.rows === 2, { failed, recovered });
  } catch (error) { failures.push({ name: 'summary accounts reports failure and recovers on retry', detail: String(error) }); }

  try {
    const holder = holdNext('models');
    await page.locator('#refresh-models').click();
    await waitEntered(holder);
    const loading = await page.evaluate(() => ({
      value: document.querySelector('#summary-models .summary-value').textContent,
      detail: document.querySelector('#summary-models .summary-detail').textContent,
    }));
    holder.release();
    await waitModelsSettled();
    const empty = await page.evaluate(() => ({
      value: document.querySelector('#summary-models .summary-value').textContent,
      detail: document.querySelector('#summary-models .summary-detail').textContent,
    }));
    modelsStatus = 503;
    await page.locator('#refresh-models').click();
    await waitModelsSettled();
    const failed = await page.evaluate(() => document.querySelector('#summary-models .summary-value').textContent);
    modelsStatus = 200;
    await page.locator('#refresh-models').click();
    await waitModelsSettled();
    const recovered = await page.evaluate(() => document.querySelector('#summary-models .summary-value').textContent);
    check('summary models shows loading, empty, failure and recovers', loading.value === '未知' && loading.detail === '正在读取模型目录' && empty.value === '0' && empty.detail === '暂无模型目录' && failed === '读取失败' && recovered === '0', { loading, empty, failed, recovered });
  } catch (error) { failures.push({ name: 'summary models shows loading, empty, failure and recovers', detail: String(error) }); }

  try {
    accounts = [];
    await page.locator('#refresh').click();
    await waitSettled();
    const empty = await page.evaluate(() => ({
      value: document.querySelector('#summary-accounts .summary-value').textContent,
      row: document.querySelector('#accounts').textContent,
      selected: document.querySelector('#g1-auth').value,
    }));
    check('summary accounts shows zero and empty table for no accounts', empty.value === '0' && empty.row.includes('暂无 Qoder 账号，请先登录。') && empty.selected === '', empty);
  } catch (error) { failures.push({ name: 'summary accounts shows zero and empty table for no accounts', detail: String(error) }); }

  if (failures.length) throw new Error('state retention regression failed: ' + JSON.stringify(failures));
  return '16 browser cases passed: drafts, focus, account selection, three stale-response paths, account removal, duplicate save, refusal retry, unsafe OAuth, delete dialog cancel/escape, summary loading/error/retry/empty';
  } finally {
    await context.close();
  }
}

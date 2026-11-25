# Qoder CPA WebUI Design System

## 0. Research Log

- Internal management surface: operational account management, not a marketing page.
- CPA management API contract: authenticated management routes and an unauthenticated resource route for the embedded page.
- Browser QA: local Playwright fixture coverage is available; production account flows remain outside this design-only check.

## 1. Direction

The page is a compact operational panel for scanning account state and changing one
transport setting. On desktop the first viewport includes the title, login action,
status feedback and account table. On mobile these follow one document scroll,
without a hero or nested cards.

## 2. Tokens

- Background: `#f5f7fb`; dark background `#101318`.
- Surface: `#ffffff`; dark surface `#191e27`.
- Text: `#182230`; dark text `#f2f4f7`.
- Muted text: `#5b6474`; dark muted text `#c0c8d4`.
- Border: `#858e9e`; dark border `#626f84`. Control edges against the panel measure 3.30:1 in light and 3.28:1 in dark.
- Primary action: `#2563eb` with white text.
- Danger action: `#b42318` with white text; dark danger action `#ff6b61` with dark text `#1b1010`.
- Free label: `#1a7f37`; dark free label `#5ee38a`.
- Promotion label: `#b45309`; dark promotion label `#f2c14e`.
- Warning states use the promotion token; normal quota states use the free token.
- Radius: 6px for controls, 8px for the single table region.
- Spacing: 8px control rhythm, 16px section rhythm, 24px page inset.

## 3. Layout

- A centered content column is capped at 1440px and uses `96vw` before the cap.
- The header contains the page title/context and the primary login action.
- Section titles keep their explanations close while model/activity controls and account load time stay on the right at wide widths; section heads become left-aligned below the 680px breakpoint.
- Account, model, and campaign tables use keyboard-focusable horizontal scroll regions below 680px, with a subtle inset edge cue and a restrained `左右滑动查看` cue below the scroller.
- No fixed or sticky panel may cover the table or status message.

Layout: The document owns vertical scrolling; `main` is a normal content container. The account, model, and campaign tables each scroll horizontally in their own `.table-wrap`. Below 680px, section heads become left-aligned and the tables retain readable column widths through local scrolling rather than clipping. Each `.table-wrap` is keyboard-focusable with `tabindex="0"` and an accurate `aria-label`. Summary groups use spacing and neutral dividers, without inner card frames.

Spacing: 在 390px、DPR 3 的触控 fixture 中，页面宽度为 390px，`main` 内边距 16px；三个表格容器各宽 308px，内容宽度为 1504px、980px、980px。1440px 和 1920px 桌面页面均无页面级横向溢出，`main` 内边距 24px。分区外距和内边距各 16px，控件间距 8px。手机控件高 44–49px，次要文字 20px。这些数据对应 Alice/Bob 两账号的测试数据，表格内容宽度随实际数据变化。

## 4. Components

- Primary button: opens CPA's native Qoder OAuth URL in a new tab.
- Status region: a live region for login, save, and error feedback.
- Account table: label, email, status, profile selector, and save action.
- Empty state: one concise sentence with the login action remaining visible.

Screen brief:

- Primary action: `添加/切换 Qoder 账号`.
- Information priorities: account state and quota, merged model catalog, then campaign/activity status.
- Left out on purpose: decorative badges, marketing copy, duplicate summary prose, and a separate dashboard shell.
- Required content: Management Key, account refresh/login controls, account status/quota/profile/priority/delete controls, merged model rates, campaign refresh/claim, activity query, and diagnostic status.

## 5. Interaction And Accessibility

- Native buttons and selects are keyboard reachable.
- Each horizontal table region is keyboard reachable with `tabindex="0"` and an accurate `aria-label`.
- Status feedback uses `role="status"`.
- Account values are HTML-escaped before insertion; delete confirmation is an in-page dialog with cancel as the default focus and a theme-aware danger action, OAuth waiting is cancelable and bounded, and account editors retain unsaved drafts and focus across refresh.
- Profile saves lock the matching row button for the request lifetime, and OAuth links accept only `http:` or `https:` URLs after parsing.
- Button press feedback uses a short filter/transform transition; `prefers-reduced-motion: reduce` removes that transition.

State table:

| State | User sees | Next action | Requests and guard |
|---|---|---|---|
| Account/model loading | `未知` summary plus a specific loading message | wait or refresh | the matching read only; no zero or empty result |
| Account/model error | `读取失败` beside the affected region | refresh | the failed region keeps its error until a new read succeeds |
| Empty model catalog | `0` and `暂无模型目录` | refresh | only after a successful zero-result response |
| Activity selection change | campaign/activity results reset to `尚未查询` | choose an account and query again | increments the G1 generation; late campaign, activity, or claim responses are dropped |
| 账号列表刷新 | 保留未保存的传输方式、优先级和活动账号选择 | 继续编辑或保存 | 重绘前记录正在编辑的字段；用户已移开的焦点不会被抢回 |
| 活动账号已删除 | 选择仍存在的首个账号，并清空旧账号结果 | 重新查询 | 原账号的活动、查询和领取响应失效 |
| 保存中或保存失败 | 保存中锁定该行按钮；失败显示原因并允许重试 | 等待或重试 | 待处理时重复点击不发送第二次保存 |
| OAuth waiting | waiting status plus `停止等待` | cancel or finish authorization | polling is bounded by five minutes and a generation token |

## 6. Responsive Behavior

- The page inset remains 24px on wide screens and becomes 16px at the 680px breakpoint.
- The table retains readable column widths through horizontal scrolling instead of clipping or overlaying content.
- Color scheme follows the operating system through `prefers-color-scheme`; dark free and promotion labels use their brighter tokens for readable contrast.
- At 680px and below, the page uses a 23px body baseline, 20px secondary roles, and 44px controls to preserve the mobile readability and touch-target floor; tables remain local horizontal scrollers.

## 7. Accepted Debt

- 本轮截图矩阵覆盖 390px 触控、1440px 和 1920px 桌面的浅深主题，包含页尾与删除对话框；额外检查 320px 长文本重排。
- 浏览器行为回归使用本地网络 fixture，不证明真实 OAuth、删除账号或权益领取成功。生产发布验收只读取管理目录和页面资源。
- The page is intentionally a single embedded HTML resource; no frontend build tool or dependency is introduced.

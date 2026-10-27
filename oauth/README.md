# CAS 单点登录接入笔记（成都理工大学 PaaS 统一认证）

> 用途：本仓库 CAS provider 的工作上下文与运维判据，防止上下文丢失。
> 范围：**只讲 CAS**。同目录其它 provider（GitHub / Discord / OIDC / LinuxDO / WeChat / Telegram / 自定义 OAuth）不在本文范围。
> 标注：✅ 已确认（读过源码或已在 prod 实测） / ❓ 待核实
> 状态：代码 ✅ 已实现并合入 main（`0027be72e`）；线上真实登录 ⏳ **待学校后台登记生效后验证**
> 约定：文中的 `<本站域名>` 指部署环境对外的公网域名，实际值不写进本仓库（例：`https://<本站域名>/oauth/cas`）。

---

## 1. CAS 服务器信息（✅ 与校内另一套已上线集成实测交叉确认）

学校：成都理工大学（cdut.edu.cn，PaaS 统一认证平台）。**标准 Apereo CAS 3.x**，非金智魔改（命名空间 `yale.edu/tp/cas`，错误码 `INVALID_TICKET` 标准）。

| 项 | 值 |
| --- | --- |
| CAS 基址 | `https://cas.paas.cdut.edu.cn/cas` |
| 登录 | `/cas/login?service=<登记的 service>` |
| 登出 | `/cas/logout?service=<登出后落地页>` |
| 票据校验 | `/cas/p3/serviceValidate?format=JSON&ticket=&service=` **（路径即协议版本，3.0 才带属性，勿改回低版本）** |
| 认证账号字段 | 响应顶层 `<cas:user>` = 统一认证账号 = **职工号** |
| 内省端点 | 无（`/cas/actuator/*`、`/cas/status` 全 404；CAS 协议没有发现文档） |
| 旁支模块 | 同时开了 `/cas/oauth2.0/*` 与 `/cas/oidc/*`（本仓库未使用，见 §9） |

学校给的参考实现放在 [`CAS对接资料/`](./CAS对接资料)（JAVA-JSP / PHP / Dotnet 三套 + 两份 docx 说明）。JSP 那套的 `supwisdom/Constants.jsp`、`supwisdom/CASUtils.jsp`、`supwisdom/LoginUser.jsp` 是最有用的三份：分别给出基址、service 与票据校验的构造方式、以及断言属性的取法。

---

## 2. 本仓库的实现位置

CAS 只实现 provider 那一半，其余全部复用上游的通用 OAuth 链路。

**后端**

| 文件 | 职责 |
| --- | --- |
| `oauth/cas.go` | `CASProvider`：`ExchangeToken` 调 `/p3/serviceValidate`（`format=JSON`）拿 principal + 属性；`GetUserInfo` 映射成 `OAuthUser`；`oauth/cas_test.go` 是它的回归测试 |
| `setting/system_setting/cas.go` | 配置结构与 URL 派生：`ServiceURL()`（**常量**）、`SignInURL()`、`ServiceValidateURL()`、`SingleLogoutURL()` |
| `controller/oauth.go` | 通用入口 `GET /api/oauth/:provider`（CAS 与其它 provider 共用；state 校验、建号/绑定都在这里） |
| `controller/misc.go` | `/api/status` 暴露 `cas_oauth` / `cas_display_name` / `cas_login_url` |
| `model/auth_flow.go` | 流程令牌（state）的签发、HMAC 存储、一次性核销 |
| `model/user.go` | `users.cas_id` 列与 `FillUserByCasId` |

**前端**

| 文件 | 职责 |
| --- | --- |
| `lib/oauth.ts` | `buildOAuthAuthorizationUrl` 的 `case 'cas'`：**直接返回后端下发的 `cas_login_url`**，前端不再自己拼 service |
| `features/auth/hooks/use-oauth-login.ts` | 登录页点按钮：建流程、存 state、跳 CAS |
| `features/auth/lib/oauth-callback-mode.ts` | state 的存取（`oauth_callback_state:{provider}`）与回调模式判定 |
| `routes/oauth/$provider.tsx` | 回调页 `/oauth/cas`：取 ticket（CAS 的票）与 state，调后端交换 |
| `features/auth/lib/oauth-popup.ts` | 绑定/安全验证用的弹窗流程，同样写入 state |
| `components/sign-out-dialog.tsx` | 读后端返回的 `sso_logout_url`，跳 CAS 单一注销 |
| `features/system-settings/auth/oauth-section.tsx` | 设置页的 CAS 标签（`/system-settings/auth/oauth`） |

---

## 3. 端到端流程（✅ 读源码）

```
浏览器                     本仓库后端                      学校 CAS
  │ 1. 点「CAS 登录」
  │ ──POST /api/oauth/state──► 建 auth_flow 记录（purpose=oauth, provider=cas）
  │ ◄── state（随机 token，库里只存 HMAC）──────
  │ 2. 把 state 存进本标签页 sessionStorage
  │ 3. 跳转（后端下发的完整地址）
  │ ─────────────────────────────────────────► /cas/login?service=https://<本站>/oauth/cas
  │ 4. 用户在 CAS 输账号密码
  │ ◄───────────────────────────────────────── 302 → https://<本站>/oauth/cas?ticket=ST-xxx
  │ 5. SPA 回调页：ticket + 从 sessionStorage 取回的 state
  │ ──GET /api/oauth/cas?code=<ticket>&state=<state>──► 校验 state（一次性核销）
  │                     GET /cas/p3/serviceValidate?format=JSON&ticket=&service=<常量>
  │                     ──────────────────────────►
  │                     ◄── {authenticationSuccess:{user, attributes}} ──
  │ ◄── 建号或命中已有 cas_id → 签发会话 ────────
  │ 6. 前台跳控制台
```

两个要点：

- **state 走两处**：后端 `auth_flow` 记录（库中只有 `token_hash`，一次性、带 TTL），以及**发起流程的那个标签页的 sessionStorage**。CAS 回调的 URL 里**没有** state（见 §4），所以回调页从 sessionStorage 取。
- **service 是常量**：登录时用的和校验时用的都由 `CASSettings.ServiceURL()` 产出，逐字节相同——CAS 会比对这两次，不一致就判票据无效。

---

## 4. 关键约束：service 必须精确匹配（这是本仓库接入时最大的坑）

学校 CAS **只放行管理后台登记的那一个 service URL，逐字节精确匹配**。任何附加查询参数——包括每次登录都不同的 `state`——都会让浏览器停在「未认证授权的服务 / not found in service registry」，**根本走不到票据校验**（所以这时后端日志里什么都看不到）。

由此推出三条硬约束：

1. **`ServiceURL()` 必须是常量**。本仓库早期把它写成 `{本站}/oauth/cas?state=<随机>`，于是永远匹配不上登记值。`0027be72e` 已改为常量 `{本站}/oauth/cas`，state 改由 sessionStorage 承载。
2. **它由「网站地址」（`ServerAddress`）派生**（`setting/system_setting/cas.go` 的 `serviceOrigin()`）。而 `ServerAddress` 的出厂默认是 `http://localhost:3000`——**没在控制台配过，service 就会被发成 `http://localhost:3000/oauth/cas`**（实测踩到）。所以部署时必须先设 `系统设置 → 站点 → 系统信息 → 网站地址` 为公网 https 地址（保存即生效，`model.UpdateOption` 同时写库与内存，不用重启容器）。顺带一提，`ServerAddress` 还被 passkey 的 RP ID 使用，本来就该是公网地址。
3. **localhost 永远无法登记**，所以本地开发环境点 CAS 登录必然失败——CAS 只能在部署环境联调。

**登记值（学校管理后台两格）**

| 字段 | 填什么 | 为什么 |
| --- | --- | --- |
| 单点登录网址 | `https://<本站域名>/oauth/cas` | 即 `ServiceURL()`，登录与校验都用它 |
| 单点注销网址 | `https://<本站域名>/sign-in` | 必须与 `SingleLogoutURL()` 发出的 `service` 一致；填回调地址会导致登出后落到"缺 ticket"的错误页 |

**登记是否生效的判据**（不依赖本仓库代码，登记完即可验）：

```bash
curl -sS --get --data-urlencode "service=https://<本站域名>/oauth/cas" \
  "https://cas.paas.cdut.edu.cn/cas/login" | grep -c "service registry"
```

输出 `0` = 已登记（返回的是真登录页）；`1` = 还没生效。等价判据是 grep 登录页表单字段 `name="password"`。

**核对本站实际会发出的 service**（改完「网站地址」后自查）：

```bash
curl -sS http://127.0.0.1:3000/api/status | grep -o '"cas_login_url":"[^"]*"'
```

期望形如 `"cas_login_url":"https://cas.paas.cdut.edu.cn/cas/login?service=..."`，其中 `service` 的值是 URL 编码后的 `https://<本站域名>/oauth/cas`。

---

## 5. 配置（设置页 ↔ option key ↔ 代码）

设置页：**系统设置 → 身份验证 → OAuth 集成 → CAS 标签**（`/system-settings/auth/oauth`）。

| 字段 | option key | 说明 |
| --- | --- | --- |
| CAS 服务器地址 | `cas.server_url` | 只填上下文根（`https://cas.paas.cdut.edu.cn/cas`），登录/登出/校验三个端点由它派生，**不要填具体端点** |
| 显示名称 | `cas.display_name` | 登录按钮文案，留空显示 `CAS` |
| 姓名属性 | `cas.name_attribute` | 留空即 `name`（代码另有 `nick` / `cn` / `displayName` 兜底） |
| 邮箱属性 | `cas.email_attribute` | 留空即 `email`（另有 `mail` 兜底） |
| 启用 CAS 登录 | `cas.enabled` | 服务端在服务器地址未配置时会**拒绝启用**（`controller/option.go`），报 `Cannot enable CAS. Enter the CAS server URL first.` |

**该校实测放行的属性里确实有 `name` 和 `email`**，所以"姓名属性 / 邮箱属性"两项**留空即可**，不要凭参考实现的字段名去猜。

---

## 6. 断言属性与 PII 纪律

- 主体取自**协议层**的 `<cas:user>`（`serviceResponse.authenticationSuccess.user`），**不从 attributes 取**：属性放行策略由学校随时可改，从属性取等于把"能否登录"押在对方配置上。
- 本 provider 只取两个属性：`name` → `OAuthUser.DisplayName`，`email` → `OAuthUser.Email`。**其余全部丢弃**（属性 map 只在本次请求的内存里经由 `OAuthUser.Extra` 传递，不落库、不写日志）。
- 该校断言含敏感 PII（实测放行约 35 个属性，其中 `idCard`、`mobile`、`tel`、`dn` 属个人信息），两个属性都取不到时只记录**属性键名**、绝不记录值。
- 属性值形状不定：单值是字符串、多值是数组（`casAttributeString` 两种都接受）。参考实现里还有个"数组包着几十个字段对象"的 `groups` 属性——不需要它，忽略。

---

## 7. 账号行为

- **首次登录**：以 `<cas:user>` 作为 `users.cas_id`。命中已有 `cas_id` 直接登录；没有则在「允许新用户注册」开启时建号——用户名优先用统一认证账号（职工号）本身，被占用或超长则退回 `cas_<自增ID>`；显示名优先取断言的 `name`，取不到则退回统一认证账号；邮箱取断言的 `email`（做归一化）。
- **不会按邮箱自动合并到已有本地账号**：若断言里的邮箱已被**另一个**账号占用，建号会直接失败并提示邮箱已被占用（`OAuthEmailAlreadyTakenError`），不会合并、也不会静默改邮箱。已有本地账号的用户想用 CAS 登录同一个账号，需要先登录后到「账号绑定」里绑定（绑定走弹窗流程，与其它 provider 一致）；否则会另开一个账号。
- **「允许新用户注册」关闭时**，未绑定过的 CAS 用户会被直接拒绝（`controller/oauth.go` 的通用建号分支）；已绑定过的用户不受影响。校内场景可以把这条当白名单用，但要清楚它的行为。
- **注销**：仅当会话是通过 CAS 建立的（登录方式记为 `oauth:cas`）时，后端在登出响应里返回 `sso_logout_url`，前端跳 `{CAS}/logout?service={网站地址}/sign-in`——即同时结束 CAS 的单一登录会话。普通密码登录的会话不会触发这一步（否则会误杀无关的 SSO 会话）。

---

## 8. 故障排查（症状 → 含义 → 下一步）

| 症状 | 含义 | 下一步 |
| --- | --- | --- |
| CAS 页显示「未认证授权的服务 / not found in service registry」 | service 未登记，或与登记值不逐字节相同 | 用 §4 的 curl 判据确认；再核对「网站地址」与控制台里的 service 值 |
| 同上，且 service 是 `http://localhost:3000/oauth/cas` | 「网站地址」没配，用了出厂默认值 | 按 §4 第 2 条设置 |
| 登录按钮根本不出现 | `cas_oauth` 为 false | 后端看 `cas.enabled` 与 `cas.server_url` 是否都就位（`/api/status`） |
| 回跳后前端提示 `Missing code` | 回调没带 ticket | 检查 CAS 是否真的跳回了登记的那个 URL |
| 回跳后 403 `State parameter is empty or mismatched` | 浏览器里没有这次流程的 state | 常见于跨标签页/换浏览器完成登录、或 sessionStorage 被清；重新点登录即可 |
| 后端日志 `CAS ticket validation failed` | 票被 CAS 拒（过期/已消费/service 不一致） | 日志里有 CAS 返回的 `code`/`description`；重点看 service 是否两次一致 |
| 登录成功但账号不对 | CAS 账号与预期不符 | 看 `users.cas_id`；主体是 `<cas:user>`，不是 attributes 里的 `staffNo`/`localAccount` |

排查时先分层：CAS 的错误页 = **第一跳就被拒**（登记问题，与本仓库代码无关）；能回到本站并带 ticket = 登记已通，问题在本仓库或票据校验这一段。

---

## 9. 已排除的替代路线

学校 CAS 同时开了 `/cas/oauth2.0/*` 与 `/cas/oidc/*`。改用本仓库**内置的 OIDC provider** 是可行的替代（它的 `redirect_uri` 本来就是常量、`state` 走标准 OAuth 参数由 IdP 回传，天然不受"service 精确匹配"限制，**零代码改动**），但没有采用：

- 按校内另一套集成的实测，**OIDC 的 claims 不含 `email`**，只有 CAS 断言有；
- 要向学校另行申请 OIDC 客户端凭据，而 CAS 应用已在管理后台建好；
- 认人字段与建号行为都要重新定义。

若将来 CAS 侧策略变化导致这条路走不通，可回头评估——届时要登记的是 `/cas/oidc/authorize` 那条链路的 redirect_uri，不要照抄 `/cas/oauth2.0/callbackAuthorize` 的形态。

---

## 10. 部署清单（每次上线照做）

1. 学校管理后台：填「单点登录网址」「单点注销网址」（§4 表格），顺手确认那格是**前向跳转**还是 CAS 主动 POST 的回调接口（本仓库实现的是前向，没有 back-channel 端点）。
2. 控制台：**网站地址**设为 `https://<本站域名>`（无尾斜杠、无路径、必须 https）；**身份验证 → OAuth 集成 → CAS** 填服务器地址并启用。
3. 拉代码重建镜像：`docker compose up -d --build`（前端产物会被编进 Go 二进制，改前端也必须重建）。
4. 自验 service：§4 的 `/api/status` 命令。
5. 线上真实登录一次：点击后回跳 URL 应是 `…/oauth/cas?ticket=ST-…`（**不带 state**），随后进入控制台。
6. 验登出：登出后应落到 `…/sign-in`，而不是缺 ticket 的错误页。

> 本地无法完成第 5 步——localhost 进不了白名单，这是校内另一套集成早有实证的结论。

---

## 11. 踩过的坑（防复发）

1. **把 state 拼进 service** → 学校是精确匹配，永远匹配不上，表现为第一跳就被 CAS 拒绝（已修，见 §4）。
2. **`ServerAddress` 的默认值**是 `http://localhost:3000`，不配就会把 service 发成 localhost；service 改由后端派生之后，这个设置从"影响邮件链接"升级成"CAS 登录的硬依赖"。
3. **本地无法联调 CAS**：不要试图在本地"先跑通登录"，只能验到回跳 URL 的形状。
4. **前端拼 service 与后端拼的不一致**会在票据校验阶段报一个很难定位的错——所以现在 service 只由后端产出（`/api/status` 的 `cas_login_url` 是完整登录地址），前端不再参与。
5. **别把主体改成从 attributes 取**（§6），也别为了"多拿信息"去全量收集属性（§6 的 PII 纪律）。

---

## 12. 未决 / 待办

- ⏳ **线上真实登录验证**：登记生效 + 新镜像上线后走一次，确认回跳 URL 形状与服务端会话都正常。
- ❓ **单一注销的语义**：学校那格若其实是 back-channel（CAS 主动 POST 通知应用），当前实现接不住。与校内另一套集成不同，本仓库有服务端会话表，理论上可以真正实现会话失效，但需要先确认学校是否强制。
- ❓ **姓名/邮箱的落点**：目前只映射到 `users.display_name` / `users.email`；是否要作为校内身份的唯一来源、要不要与其它校内系统对齐，尚未决定。

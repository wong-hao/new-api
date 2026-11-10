# 新消息中心（htmq）对接说明

来源：校方提供的《新消息中心对接文档》.docx。本文件是其转写，并补充了实现要点与待确认项。
接口路径前缀为 `/htmq/`，因此对应的 Go 包命名为 `msgcenter`。

因发送消息采用接口方式，在发消息时，直接调用接口即可。共两个接口：

| 用途 | 接口 |
| --- | --- |
| 查询授权应用可发送的渠道 | `GET {base}/htmq/getChannels` |
| 发送消息 | `POST {base}/htmq/sendMsg` |

`{base}` 是部署地址（见「与原文不一致 / 待确认」第 1 条）。代码中不写死，由调用方传入。

## 通用约定

- 鉴权：每个请求都在请求头带 `appId`（授权应用 appId）与 `token`（令牌，原文写「暂时为随机串」）。
- 返回信封：`{"code": 200, "message": "成功", "data": ...}`。
  原文的字段说明把消息字段写作 `msg`，而两个示例里返回的都是 `message`；实现里两种都解析，优先 `message`。
- `code == 200` 为成功，非 200 见文末返回码表。

## 1. 获取授权应用的渠道信息

- 请求：`GET {base}/htmq/getChannels`
- 参数位置：请求头 `appId`、`token`（原文「参数」一节写的是「位置：请求头中」）
- 返回 `data`：

| 字段 | 说明 |
| --- | --- |
| `channels` | 该应用可发送的渠道，英文逗号分隔 |
| `validDate` | 授权有效期，形如 `2021-10-21 08:00:00 - 2023-12-31 08:00:00` |

渠道取值：`sms`、`email`、`wechat`、`dingding`、`eip`、`app`。

示例（据原文截图转写，示例值已替换为占位）：

```
GET http://<base>/htmq/getChannels
appId: <授权应用id>
token: <自定义随机字符串>

{
  "code": 200,
  "message": "成功",
  "data": {
    "channels": "sms,wechat",
    "validDate": "2021-10-21 08:00:00 - 2023-12-31 08:00:00"
  }
}
```

## 2. 发送消息

- 请求：`POST {base}/htmq/sendMsg`
- 参数位置：请求头 `appId`、`token`
- 请求体：`Content-Type: application/x-www-form-urlencoded`，唯一字段 `msgData`，其值是消息对象的 JSON 字符串（见原文截图的 Body 面板）。原文「参数」一节只写了「`msgData`【位置：请求体】」。

`msgData` 字段：

| 字段 | 说明 | 必填条件 |
| --- | --- | --- |
| `title` | 标题 | |
| `content` | 消息内容 | |
| `channels` | 消息渠道（`sms`、`email`、`wechat`、`dingding`） | |
| `intReceiver` | 内部接收人（学工号），英文逗号分隔 | 渠道含 `wechat`、`dingding`、`eip`、`app` 时不为空 |
| `smsReceiver` | 短信接收人，英文逗号分隔 | 渠道含 `sms` 时不为空 |
| `emailReceiver` | 邮件接收人，英文逗号分隔 | 渠道含 `email` 时不为空 |
| `wechatMsgType` | 微信消息类型：文本 `text`、图文 `news`、文件 `file`、文本卡片 `textcard` | 渠道含 `wechat` 时必填 |
| `dingMsgType` | 钉钉消息类型：文本 `text`、图文 `link`、文件 `file`、文本卡片 `action_card` | 渠道含 `dingding` 时必填 |
| `moduleID` | 消息来源：流程 `COOPERATE`、通知 `ANNOUNCEMENT`、调查 `SURVEY`、会议 `MEET`、第三方 `JOIN_APP` | 渠道含 `eip` 时必填 |
| `btntext` | 描述文本，例如「更多」「查看详情」 | |
| `wechatAgentId` | 接收消息的企业微信应用 AgentId | 为空则使用服务端配置 |
| `dingAgentId` | 接收消息的钉钉应用 AgentId | 为空则使用服务端配置 |
| `cron` | 定时发送：`1` 定时、`0` 即时 | |
| `sendTime` | 发送时间，格式 `yyyy-MM-dd HH:mm:ss` | `cron` 为 `1` 时必填 |
| `pcUrl` | PC 端点击消息时的链接 | |
| `mobileUrl` | 手机端点击消息时的链接 | |
| `fileId` | 文件 ID（调用上传接口返回）。图片 < 2M，普通文件 < 20M | 为空则按消息类型用服务端默认配置 |
| `template` | 消息模板，JSON 字符串，key 为渠道、value 为模板 Code，如 `"{\"email\":\"emailCode01\"}"` | |

- 返回 `data`：`msgId`（消息 id，字符串）

示例（据原文截图转写，示例值已替换为占位）：

```
POST http://<base>/htmq/sendMsg
appId: <授权应用id>
token: <自定义随机字符串>
Content-Type: application/x-www-form-urlencoded

msgData={"title":"ces","content":"111111","channels":"wechat","intReceiver":"<学工号>",
"smsReceiver":"","emailReceiver":"","wechatMsgType":"textcard","dingMsgType":"","moduleID":"CSXT",
"btntext":"111111","wechatAgentId":"","dingAgentId":"","cron":"0","sendTime":"","pcUrl":"www.baidu.com",
"mobileUrl":"www.baidu.com","fileId":"","template":""}

{
  "code": 200,
  "message": "成功",
  "data": "<msgId>"
}
```

## 返回码表

| code | message |
| --- | --- |
| 200 | 成功 |
| 400 | Bad Request |
| 500 | Internal Server Error |
| 6010130 | 访问超时 |
| 6010401 | 请求的数据格式不符 |
| 6010403 | 未经授权访问 |
| 6010403 | 拒绝访问 |
| 6010404 | 无法找到资源 |
| 6010405 | 不支持当前请求方法 |
| 6010503 | 服务器正忙，请稍后再试 |
| 6010520 | 数据校验不通过的相关提示 |
| 6010521 | 缺少必要参数 |
| 6010522 | ContentType 格式不正确 |
| 6010525 | 修改的对象为空或缺少主键 |
| 6010526 | 操作失败,请刷新后重试 |
| 6010527 | 请检查分页参数[page,limit] |
| 6010530 | 请求HTTP接口异常 |
| 6010560 | 账户或密码不正确 |
| 6010561 | 验证码不正确 |
| 6010562 | 没有查询到登录用户信息 |
| 6010601 | 已存在 |
| 6010602 | 根据{0}[{1}]没有查询到相关信息 |
| 6010603 | 没有开通渠道权限 |
| 6010604 | 无效的应用(appId不正确、不在有效期内、已停用) |
| 6010605 | 消息体格式错误 |
| 6010606 | 包含敏感词:{0} |
| 6010607 | 系统没有配置渠道信息 |
| 6010610 | 发送邮件失败 |
| 6010611 | 发送短信失败 |
| 6010612 | 发送微信失败 |
| 6010613 | 发送钉钉失败 |
| 6010614 | 发送EIP失败 |
| 6010615 | 发送APP失败 |
| 6010620 | 消息接收人为空，无法发送 |
| 6010621 | 配置不在配置列表中，无法发送 |
| 6010622 | 后台应用配置不在配置列表中，无法发送 |
| 6010623 | 没有查询到{0}图文配置信息，无法确定接收消息的应用，无法发送 |
| 6010624 | 今日发送消息次数已达上限，无法发送 |
| 6010625 | 获取token失败，无法发送 |
| 6010626 | 发送消息返回结果为空 |
| 6010627 | 消息体数据为空，发送失败 |
| 6010628 | 模板不能为空/模板[{0}]不在配置列表中，无法发送 |
| 6010629 | 不包含有效的手机号码，无法发送 |
| 6010630 | 不包含有效的邮箱，无法发送 |
| 6010631 | 不包含有效账号，无法发送 |
| 6010650 | 文件为空，上传失败 |
| 6010651 | 没有查询到文件信息 |
| 6010652 | 上传文件大小超出限制 |
| 6010653 | 上传文件总大小超出限制 |

`{0}`、`{1}` 是原文保留的占位符，由服务端填充。

## 与原文不一致 / 待确认

1. **部署端口**：正文地址写 `...:9200`，两张示例截图都是 `...:8080`。排查时以实际部署为准，因此代码不写死 base URL。
2. **getChannels 参数位置**：正文写「位置：请求头中」，但地址示例又写成 `?appId=账户&token=令牌` 的查询串。当前实现按请求头发送（与截图一致）；若服务端只认查询串，需要再改。
3. **消息字段名**：正文写 `msg`，示例返回 `message`。实现两个都解析。
4. **sendMsg 请求体**：正文只说「`msgData`【位置：请求体】」，截图的 Body 是 `x-www-form-urlencoded` + 单个 `msgData` 字段。当前实现按截图；若服务端也接受原始 JSON 请求体，可再简化。
5. **`cron` 类型**：正文描述为 `1`/`0`，截图里是字符串 `"0"`。实现按字符串传，未填时补 `"0"`（即时发送）。
6. **`channels` 是应用级授权**：截图示例只返回 `sms,wechat`，即返回的是该 appId 开通的渠道，不代表系统支持的全部渠道。上线实测的一个应用返回 `wechat,eip`——没有钉钉，所以默认渠道不能是 `wechat,dingding`。
7. **`getChannels` 的实际返回比文档多**：实测还带 `startValidDate`、`endValidDate`、`status`（`"1"`），文档只写了 `channels` 和 `validDate`。客户端只取后两者，其余忽略。
8. **`eip` 需要 `moduleID`**：它既要按学工号投递，又必须带消息来源，所以站点配置里多了一个 `msgcenter.module_id`（没配就不发 eip，其余渠道照发）。`app` 渠道只需要学工号，已一并支持。
9. **未说明**：是否提供 HTTPS、是否有来源 IP 白名单、`token` 是否会轮换及轮换流程、单条消息的接收人数量/频率上限（返回码里有「今日发送消息次数已达上限」但未给配额）。
10. **接收人字段与渠道的对应**：`intReceiver` 覆盖 `wechat`、`dingding`、`eip`、`app` 四个渠道，短信与邮件各有独立字段。

## 实现说明

同目录代码：`client.go`（传输与两个接口）、`types.go`（DTO、渠道取值、错误类型）、`client_test.go`（线格式契约测试）。

- 包只做「传输 + DTO + 返回码映射」，不读配置、不写库、不碰通知业务；`appId` / `token` / base URL 由调用方传入，后续接 `setting/` 时再决定配置项归属。
- 发送失败返回 `*APIError`，带上 `code` 与 `message`，便于按返回码分支（重试、告警、降级）。
- 原文两张示例截图未随本文件提交：`intReceiver` 里是真实学工号，且截图含内网地址。示例已按上文的转写形式保留，示例值全部替换为占位。
- 尚未接线：把告警真正落到这个渠道的其他场景、失败重试与限流策略。

## 在本仓库中的接线

### 站点配置（管理端「系统设置 → 运营设置 → 消息中心」）

配置组 `msgcenter`，落地在 `setting/system_setting/msgcenter.go`，option 键：

| 键 | 说明 |
| --- | --- |
| `msgcenter.enabled` | 总开关，关掉后所有 OA 消息都不发 |
| `msgcenter.base_url` | 部署地址，**含端口**（正文 9200 与示例 8080 不一致，故不设默认值），必须是绝对 http/https URL |
| `msgcenter.app_id` | 授权应用 appId |
| `msgcenter.token` | 令牌 |
| `msgcenter.channels` | 本站点发送使用的渠道，逗号分隔，取值 `wechat`、`dingding`、`eip`、`app`；留空只用 `wechat` |
| `msgcenter.module_id` | EIP 的消息来源，取值 `COOPERATE`、`ANNOUNCEMENT`、`SURVEY`、`MEET`、`JOIN_APP`；选中 eip 时必填 |

四个值齐备才算「已配置」，否则 `sendOANotify` 直接跳过并记一行日志。token 是密钥，只出现在 root 权限的 option 接口里，不进公开的 `/api/status`（那里只有一个布尔 `msgcenter_configured`，供控制台决定是否提供 OA、以及默认通知方式）。

配置只从这两处落地：管理端界面的这四个 option，或直接写同名的 option（`/api/option/`）。没有环境变量兜底。多节点部署下，写入的那个节点立即生效，其余节点在 `SYNC_FREQUENCY`（默认 60 秒）的选项同步里跟上，不需要重启。

### 用户侧

- 通知方式新增 `oa`（`relaykit/dto` 的 `NotifyTypeOA`），个人设置 → 通知里可选，选后出现渠道多选。
- **默认通知方式**：站点配好消息中心后，从未选过通知方式的用户默认走 OA（后端 `deliveryMethod`，前端 `defaultNotifyType`，都以 `msgcenter_configured` 为准）；没配好时默认仍是 email，且控制台不提供 OA 这一项——未配置的中心会收下选择却什么都发不出去。
- `oa_channels` 存逗号分隔的渠道，取值来自站点配置（`/api/status` 的 `msgcenter_channels`）——用户只能在站点要发的渠道里再窄选，因为消息中心按应用授权，带上未开通的渠道会让**整条消息**失败（`6010603`）。留空表示用站点配置的渠道。前后端都做同样的过滤（后端 `messageChannels`，前端 `normalizeOAChannels`）。
- **每个渠道各有必填字段**，发送前按渠道列表补齐，否则整条消息会被拒：`wechat` → `wechatMsgType`（用 `text`）、`dingding` → `dingMsgType`（用 `text`）、`eip` → `moduleID`（取站点配置，没配就把 eip 从本次发送里去掉）、`app` → 只需学工号。
- **收件人**：该用户的 `User.CasId`，即 CAS 登录的统一账号（学工号）。没有绑定过 CAS 的账号（例如 root）不发，记日志跳过。
- 消息类型固定 `text`，充值链接写在正文里；`fileId`、`template`、`eip`/`sms`/`email` 渠道本次不使用。

### 触发：余额为 0 导致扣费失败

- 位置：`service/billing.go` 的 `PreConsumeBilling`——预扣费的统一入口，用 defer 检查它每一个出口，因此只在请求**最终**被拒时触发（`wallet_first` 回退订阅后成功不会误报）。
- 条件：错误码为 `insufficient_user_quota` 且该用户钱包额度 ≤ 0，且非 playground。
- **必达**：这一条不看站点开关 `QuotaRemindEnabled`，也不看用户的「低额度提醒」开关。那两个开关的语义是「余额跌破阈值时提醒一次」，而这里报的是已经失败了的请求，所以选了消息中心的用户必须收到。要停掉它，把消息中心总开关关掉即可（那是有意为之的唯一闸门）。
- 投递：走用户自己选的通知方式（`NotifyUser`）。
- 独立限流：`Notify.Type` 用 `quota_empty`（不是低额度提醒的 `quota_exceed`），因此有自己的一份 `CheckNotificationLimit` 预算（默认每用户 2 条 / 10 分钟），不会被低额度提醒用掉；余额为 0 的客户端重试也不会刷出成串消息。
- 异步（`gopool.Go`）、只读：不阻塞响应、不参与任何 quota 运算，失败只记 `SysError`。
- 文案：`i18n/locales/{en,zh-CN,zh-TW}.yaml` 的 `notify.quota_empty_*`，按用户语言渲染；OA 与 Bark/Gotify 用纯文本，email/webhook 用带链接的 HTML。
- 未覆盖：已有会话上的补预扣失败（图片调价、任务换更贵渠道时 `Billing.Reserve`）不触发——余额为 0 的用户首次预扣已经失败，走不到那里。

### 为什么不用 SSRF 防护客户端

消息中心是校内地址（`202.115.x`），用 `GetSSRFProtectedHTTPClient()` 会被内网拦截；`pkg/msgcenter` 自带 `http.Client`，默认 30 秒超时，不经过 SSRF 校验，也不走 worker 转发。

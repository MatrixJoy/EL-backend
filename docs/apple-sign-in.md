# Apple 登录接入与撤销

更新：2026-09-11。App ID：`cn.wozdou.ela`；团队：`34L323UH86`。

## 已实现

- `GET /api/v1/auth/capabilities` 返回实际登录能力和 Client ID，禁止缓存。全部 Apple 配置有效才开放 Apple 登录。
- `POST /api/v1/auth/apple` 要求 `identityToken`、一次性 `authorizationCode`、`nonce`，可携带原有 `migration`。先验证签名、issuer、audience、nonce，再向 Apple 换取 token，并核对两份身份的 subject 与 nonce 一致。会话返回 `appleUserId`。
- 刷新凭据用 AES-256-GCM 加密并绑定凭据记录 ID，与用户、迁移数据和会话一起事务入库。不记录原始 code、token、私钥或 Apple 错误响应正文。
- 删除账号时，外键自动将凭据转为脱离用户的撤销任务；账号和会话删除不依赖 Apple 在线。API 每分钟处理持久队列，行锁防止多副本重复领取；失败指数退避，上限约 17 小时，不丢弃请求。Apple 确认撤销后删除密文。
- `docker exec english-learning-api admin apple-auth-status` 查看活跃凭据、待撤销和重试数量，不输出凭据。内网替换为实际 API 容器名。
- iOS 前台恢复及 Apple 撤销通知会检查系统 credential state；确认 revoked/notFound 后清除本机会话、退出服务端会话，保留学习资料。临时系统错误不会误登出。

## 必须配置的凭据

应用签名证书不是 Sign in with Apple 的服务器密钥。需要 Apple Developer 中启用 **Sign in with Apple** 并关联此 App ID 的 `.p8` Key；App Store Connect API Key 不能替代。

| 环境变量 | 内容 |
| --- | --- |
| `LEARNING_APPLE_CLIENT_ID` | `cn.wozdou.ela` |
| `LEARNING_APPLE_TEAM_ID` | `34L323UH86` |
| `LEARNING_APPLE_KEY_ID` | Apple 为该 `.p8` 分配的 Key ID |
| `LEARNING_APPLE_PRIVATE_KEY_BASE64` | `.p8` 完整内容的单行 Base64 |
| `LEARNING_APPLE_TOKEN_ENCRYPTION_KEY` | 随机 32 字节的 Base64，和签名私钥分开保存 |

生产使用 `.env.production`，内网使用 `.env`。后四项必须一起配置；全部为空时继续提供匿名/内网测试账号，部分配置或格式错误则拒绝启动。环境文件保持 600，不提交到 Git；私钥不放进 App、下载目录、构建产物或聊天。凭据加密密钥需独立加密备份；不要直接替换已有数据使用的加密密钥，轮换需要迁移密文。

建议先仅在生产配置 Apple OAuth；内网沿用测试账号。同一 App ID 的测试/生产共享 Apple 授权范围，测试环境撤销可能影响生产授权。

部署后核对 capabilities 的 `appleSignInEnabled=true` 与 Client ID，再在真机验证登录、跨设备同步、删除及撤销队列归零。当前通过模拟 Apple 端测试，不能代替真实密钥联调。

## 仍需验收

Apple server-to-server 通知和后台周期刷新凭据验证尚未接入；当前在 App 前台检查系统撤销状态，不声称后台实时收到外部撤销事件。正式启用前需确认运营处理方案。撤销失败持续保留加密任务，运营可用上述命令检查；外部告警接收方仍待配置。

参考：[授权码验证](https://developer.apple.com/documentation/signinwithapplerestapi/generate-and-validate-tokens)、[Token 撤销](https://developer.apple.com/documentation/signinwithapplerestapi/revoke-tokens)、[账号删除技术说明](https://developer.apple.com/documentation/technotes/tn3194-handling-account-deletions-and-revoking-tokens-for-sign-in-with-apple)。

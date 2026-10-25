# 发布新版本

健康足迹不上 Mac App Store，发的是 Developer ID 签名、苹果公证过的 dmg。别人下载后双击就能打开，不会弹「无法验证开发者」。

## 平时：打个带说明的标签就行

标签里写的说明，就是 Release 页面上的更新说明。先看看上个版本以来改了什么：

```bash
git log --oneline $(git describe --tags --abbrev=0)..HEAD
```

用大白话写成说明，打进标签再推送：

```bash
git tag -a v1.0.1 -m "健康足迹 1.0.1

- 新增：日历里的链接能直接打开那条预约
- 修复：……"
git push origin v1.0.1
```

一定要用 `-a`（带说明的标签）：不带说明的话，Release 页面上只会显示最后一次提交的提交说明。也可以直接跟 Claude 说「发个 1.0.1」，它会先写好说明给你确认，再打标签。

GitHub Actions（`.github/workflows/release.yml`）会跑测试，打一个同时支持 Apple 芯片和 Intel 的 dmg，签名、交给苹果公证，再跑一遍 MCP 端到端测试，最后建一个 GitHub Release 把 dmg 挂上去。版本号直接取标签：`v1.0.0` → `1.0.0`。

## App 里的自动更新

Release 上除了 dmg，还有一个 `HealthTrail-<版本>.zip`：订了公证票据的同一个 .app。装好的 App 每天查一次 `releases/latest`（不含 pre-release，所以带连字符的测试标签不会推给用户），发现更新就在「设置 → 软件更新」和菜单栏里提示。点「更新并重启」后：先备份一次（设置了备份的话）→ 下载 zip → 校验大小和 SHA-256 → 检查是同一个团队的 Developer ID 签名、bundle id 一致、经过公证、版本号对得上 → 替换「应用程序」里的 App → 退出后重新打开。

- **仓库必须是公开的**：App 不带令牌，私有仓库的 Release 它读不到（显示「还没有找到发布的版本」）。
- 1.0.0 的 Release 没有 zip，App 会显示新版本，但只能点「打开下载页面」手动装。
- 自己 `make app` 构建的版本是临时签名，不会自动更新，只提示有新版本。
- 用户把 App 放在「应用程序」以外、用户没有写权限的地方，或者直接从 dmg 里运行，也只能手动装。

每次推送分支跑的是 `ci.yml`：测试，加一个临时签名的 dmg（只能自己测，别的 Mac 打不开）。

## 第一次：把签名和公证用的东西放进 GitHub Secrets

下面这些都是你自己的凭证，只在你自己的电脑上操作，不要发给任何人。

**1. 证书（Developer ID Application）**

1. 打开「钥匙串访问」→ 左边选「登录」→ 上面选「我的证书」。
2. 右键「Developer ID Application: 你的名字 (团队 ID)」→「导出…」，格式选 .p12，存到桌面，设一个密码。
3. 在终端运行（第二条会提示你输入刚才设的密码）：

   ```bash
   base64 -i ~/Desktop/developerid.p12 | gh secret set DEVELOPER_ID_P12
   gh secret set DEVELOPER_ID_P12_PASSWORD
   ```
4. 这个 .p12 同时也是证书私钥的备份：存到安全的地方，比如加密的 U 盘或密码管理器，然后从桌面删掉。换电脑或者重装系统，靠它恢复签名能力。

**2. 公证用的 App Store Connect API 密钥**

1. 打开 https://appstoreconnect.apple.com/access/integrations/api 。第一次用要先点「请求访问」并同意条款。
2. 在「团队密钥」里点「+」，名称填 `health-trail notary`，访问权限选「开发者」（Developer），然后生成。
3. 下载 `AuthKey_XXXXXXXXXX.p8`。**只能下载一次**，下载后好好保存。页面上会显示「密钥 ID」，列表上方有「Issuer ID」。
4. 在终端运行：

   ```bash
   gh secret set NOTARY_KEY < ~/Downloads/AuthKey_XXXXXXXXXX.p8
   gh secret set NOTARY_KEY_ID --body 密钥ID
   gh secret set NOTARY_ISSUER_ID --body IssuerID
   ```

用 `gh secret list` 检查，应该有这 5 个：`DEVELOPER_ID_P12`、`DEVELOPER_ID_P12_PASSWORD`、`NOTARY_KEY`、`NOTARY_KEY_ID`、`NOTARY_ISSUER_ID`。

## 在本机打包（可选）

先把 `local.mk.example` 复制成 `local.mk`（不会提交），在里面填上自己的证书名（`security find-identity -v -p codesigning` 能查到）。

```bash
make dmg         # Developer ID 签名的 dmg，在 dist/ 里，还没公证
make notarize    # 再交给苹果公证；需要先保存一次凭证：
                 # xcrun notarytool store-credentials health-trail --team-id <你的团队 ID>
```

没有 `local.mk` 时 `make dmg` 用临时签名，只能自己测试用。

第一次签名时，系统会问 codesign 能不能用钥匙串里的私钥：输入 Mac 登录密码，点「始终允许」。

## 证书到期

Developer ID Application 证书的到期日，在「钥匙串访问」里双击证书就能看到。到期前，按上面的方法重新申请一张，再更新 `DEVELOPER_ID_P12` 和 `DEVELOPER_ID_P12_PASSWORD`。已经发出去的版本不受影响：签名带了时间戳，公证也有记录。

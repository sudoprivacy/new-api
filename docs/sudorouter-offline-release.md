<!-- sudoapi: Native offline SudoRouter releases. -->

# SudoRouter 离线发布

`sudorouter-release.yml` 在原生 Linux amd64 / arm64 runner 上分别构建现有 new-api Dockerfile。在隔离的 Git commit 构建目录中写入 tag 版本作为 `VERSION`，供后端和前端共同读取。每个镜像须通过架构检查和隔离 SQLite 实例的 `/api/status` 检查，且运行时版本必须与发布版本完全一致，之后才导出 Docker 离线包。

本地构建固定使用当前 `HEAD` 的已提交源码（需要 Git、Docker、Buildx、Python 3、shasum）：

```bash
bash scripts/package-sudorouter.sh sudorouter-v0.0.1-rc.2 arm64 dist/sudorouter
```

产物为 `sudorouter-<version>-linux-<arch>.tar.gz`、对应 `.source.json` 和 `SHA256SUMS`。镜像标签为 `docker.io/sudoprivacy/new-api:<version>-<arch>`；来源文件记录实际源码 commit 和镜像 ID。amd64、arm64 的包可在同一 Release 并存。

先提交代码并创建面向 `sudo/rc24` 的 PR；PR 的 amd64、arm64 原生构建和版本检查通过后合并。手动运行 workflow 只生成候选 artifact。然后从合并 commit 推送新的 `sudorouter-v*` tag 后，两种架构都验证成功才创建 Release；已有 Release 不覆盖，也不会改动仓库的 latest Release。Sudostack 应固定这个新 tag、源 commit 和发布附件 SHA256，不得把旧 amd64 包重新标注为 arm64。上游名称、许可证和一般 Docker 发布流程保持兼容。

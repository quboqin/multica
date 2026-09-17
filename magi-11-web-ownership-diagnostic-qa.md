## 总体结论

有条件通过（诊断补丁与本地模拟验收）。提交 `217cf4b238a71d30ea3b89394c106f5f391f3c2b` 已推送到 `feature/cortex-g1-collections`；产品阻塞缺陷 0 个。

条件：当前 3.8 GiB 主机未启动 Web，未停止 API/PG/Desktop，未清理既有证据；真实 Web 启动链、扩容后的 5 分钟观察窗和 T2 业务门禁仍未验证。T2 继续不通过，T3 未启动。

## 覆盖矩阵

| 需求/场景 | 用例 | 结果 | 判定依据 |
| --- | --- | --- | --- |
| 诊断字段 | 白名单进程、端口、父链、资源字段；不记录命令行/环境 | 通过 | `scripts/dev-env.sh:496`、`scripts/dev-env.sh:522`、`scripts/dev-env.sh:569` |
| 失败时机 | 归属失败日志先于原停止路径 | 通过 | `scripts/dev-env.sh:763`、`scripts/dev-env.test.sh:242` |
| 合法父链 | launcher → 多级 listener 父链 | 通过 | `scripts/dev-env.test.sh:407` |
| 外来端口 | 当前 listener 不归属本 launcher | 通过 | `scripts/dev-env.test.sh:409` |
| 父链断开 | listener 直接回到 PID 1 | 通过 | `scripts/dev-env.test.sh:411` |
| launcher 退出 | pid 文件仍在但 `kill -0` 失败 | 通过 | `scripts/dev-env.test.sh:413` |
| listener 更替 | 记录 PID 与当前 PID 不同 | 通过 | `scripts/dev-env.test.sh:415` |
| 查询失败/无 listener | lsof 失败、返回错误和空结果 | 通过 | `scripts/dev-env.test.sh:417`、`scripts/dev-env.test.sh:419` |
| 每次独立日志 | 连续两次失败生成两个不同文件 | 通过 | `scripts/dev-env.test.sh:293` |
| 停止边界 | 外来 listener 不接收信号 | 通过 | `scripts/dev-env.test.sh:132`、`scripts/dev-env.test.sh:442` |
| ready、`CLEAN_ENV`、端口隔离 | 静态差异核对 | 通过（静态） | 只在原归属失败分支前增加 best-effort 诊断调用；判定、`CLEAN_ENV`、端口分配和 `stop_component` 未改 |

## 缺陷清单

无已复现产品缺陷。

环境阻断 1 项：3.8 GiB 主机不满足本次 Web 真实启动/观察条件，故未把模拟用例结果冒充真实就绪。

## 埋点/诊断核验结果

本任务无产品埋点；诊断日志每次使用独立临时文件，记录 schema、时间、源码 SHA、launcher/listener PID 与存活状态、PPID/PGID/RSS/comm、session/start 查询结果、cwd/exe、完整 listener PID 查询、父链、MemAvailable/Swap/OOM/pressure 查询状态与错误。查询失败保留状态码和压缩后的错误，不输出命令行或环境变量。

## 未覆盖项与原因

- 未在当前主机启动 Web、未增大堆、未停止现有 API/PG/Desktop；遵循触发评论的资源与证据保护约束。
- 未在扩容主机验证真实 launcher → pnpm/Turbo → Next listener 的父链、listener 更替竞态、Web 请求持续 5 分钟稳定性和 API/Web commit 对齐。
- 未运行 T2 Playwright 业务验收；Chromium/后端/Stage 真实条件须在扩容后按入口复验。

## 扩容后复验入口

在至少 4 vCPU / 8 GiB 的独立主机、固定本 SHA 的 checkout 中执行；不要复用或清理当前证据目录：

```bash
git rev-parse HEAD
make up C=api,web ARGS="--name magi11-web-diagnostic"
make status ARGS=--json
```

先核对 API `/health` 的 commit/pid/started_at、Web 实际 cwd/exe、端口 listener 和 `logs/web-ownership.*`；合法父链必须通过，外来端口/父链断开/launcher 退出/listener 更替/查询失败必须拒绝 ready 且不停止外来进程。环境稳定后再执行：

```bash
make env-exec ARGS="-- pnpm exec playwright test e2e/collection-table.spec.ts e2e/issue-table.spec.ts --project=chromium --trace=retain-on-failure"
```

T2 全部门禁通过前不启动 T3。

## 本地执行结果

```text
bash scripts/dev-env.test.sh                         PASS（连续 3 次）
bash -n scripts/dev-env.sh                           PASS
bash -n scripts/dev-env.test.sh                      PASS
git diff --check                                     PASS
```

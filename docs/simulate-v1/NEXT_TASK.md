# 下一步：把已验证改动保存到个人 fork，并精简 push CI

这是用户于2026-10-11批准的新流程，替代尚未执行的STAGE7F_CONSOLIDATED_DELIVERY_PROMPT.md。本任务允许在主工作区集成已经评审的改动、按逻辑提交并推送个人fork，以及由push自然触发的定向CI。不要再执行旧任务中“每轮复制全源码、全部哈希、禁止提交”的流程，也不要修改历史报告。

## 1. 工作范围与开工检查

唯一工作目录：`D:/Geth-newfunc-debug/go-ethereum`，先读此目录AGENTS.md和docs/simulate-v1/WORKLOG.md。分支为feat/simulate-v1-tracing，唯一push仓库git@github.com:afhuuuu/go-ethereum.git，upstream push保持disabled://upstream-push。本轮不修改master、不创建上游PR、不合并、不强推。

先查看git status、当前HEAD、index和全部有效push URL。规则/文档提交由评审对话先行保存，当前HEAD不再要求等于c9a2bc73；后者只是源码原始基线。若index含别人未解释的改动，先分清归属，不清空它。不要reset、stash、checkout覆盖现有修改。

以下路径中P代表`D:/Geth-newfunc-debug/geth-simulate-v1-planning`，只读来源为：

- K：P/stage7a-final-candidate，十二文件候选及五组补丁。
- C：P/stage6c-full-state-corrected/source，已验证的辅助修正来源。
- D：P/stage7d-transaction-coverage，三文件交易工具补丁及source。
- N：P/stage7e-ordinary-transaction-resume，普通交易回归报告。

读取上述MANIFEST/REPORT等小文件即可。不要运行旧runner、review.py或打包脚本，不复制整套源码/fixture、不扫描数千文件或旧大日志、不创建新的stage目录。旧副本只作为本次有限迁移的来源，后续开发留在主工作区。

## 2. 保存现有功能和辅助修正，严格分组

功能组：主工作区已有以下六个文件，保持当前实现，不重新开发：

```text
internal/ethapi/api.go
internal/ethapi/simulate.go
internal/ethapi/simulate_tracing.go
internal/ethapi/simulate_tracing_test.go
eth/api_backend_simulate.go
eth/simulate_tracing_test.go
```

只核对这六份文件与K/MANIFEST.json目标一致，并查看diff。原生tracer、debug_traceCall、core/EVM、OnChainTest.py、依赖及receipt扩展均不在修改范围。internal/ethapi/logtracer.go的实际内容diff应为空，不要为了清理status改写它。

已有辅助修正也要留下可回档记录，但与功能独立提交：

1. C/accounts/usbwallet/trezor/trezor.go：只拆分既有生成命令的include参数。
2. C/cmd/utils/diskusage_windows.go：只删除已定位的多余空行。
3. C/tests/state_test.go：只沿用已评审的不存在目录项修正，不更改Legacy/spec过滤或预期。
4. D/source/tests/transaction_test_util.go、tests/transaction_test.go、tests/transaction_fork_coverage_test.go：沿用Stage7D已验证的三文件工具修正及回归。

每组修改前比较主目录当前内容与固定原始基线/预期目标，避免覆盖新改动。若已是目标则不重复处理；如果出现其他实质差异，停止该组并说明。可以直接以bytes复制这六个指定文件，或者对主目录应用对应已评审补丁，二选一，不做新apply-check仓库。迁移后只比较这些文件的diff和目标内容；区分CRLF/LF与语义变化，不批量转换全库行尾。

建议提交主题（每组独立，确有改动才提交）：

```text
internal/ethapi, eth: add optional tracing to simulation
accounts/usbwallet/trezor: split protobuf include paths
cmd/utils: remove redundant blank line
tests: remove obsolete state test directory
tests: cover omitted transaction forks
```

这些辅助修正进入个人fork，不自动意味着未来上游功能PR也要包含它们。

## 3. 精简个人 fork workflow

仅修改下面三份现有workflow的必要部分；go.yml和validate_pr.yml已添加的上游仓库限定应保留：

```text
.github/workflows/go.yml
.github/workflows/validate_pr.yml
.github/workflows/simulate-v1.yml
```

重点调整simulate-v1.yml：

- 普通push和fork内PR只运行已有Linux/Windows定向模拟、相关原生tracer回归、Linux生命周期/隔离race及geth构建。保留测试名称存在性检查，不弱化断言，不用continue-on-error掩盖失败。
- Markdown/文档专用修改不触发Go jobs；给push和pull_request都配置适当路径过滤。workflow本身、相关Go源码或模块依赖改变时仍应触发。
- 新增workflow_dispatch，使用布尔输入full_validation，默认false。现有quality、全库多版本、keeper和386等重型jobs，仅在本仓库且显式手动选择full_validation=true时运行。检查needs/if依赖，确保普通push不会穿透到重型jobs，手动full=true仍能按顺序运行。
- 保留contents:read、个人仓库限制、PR来源限制、checkout不持久化凭据、工具版本、超时和并发取消等现有有效配置。不要换self-hosted runner，不扩展权限，不增加部署或评论动作。
- 不执行workflow_dispatch，不运行本地全库/生成/多平台矩阵。用户已授权普通push所自然触发的定向CI；不代表授权重型手动运行。

CI组独立提交，主题可用`.github/workflows: separate checkpoint and full validation`。

## 4. 验证只覆盖本轮真实变化

六份核心文件未改动时复用Stage2E功能/race和Stage4C相关API证据；迁入辅助修正与既有来源一致时，复用对应Windows检查、Stage6C、Stage7D及Stage7E结果，明确它们是历史验证。不要为了换目录或保存Git提交重复上述Go测试。

如果迁移发生了新的源码语义修改，这超出“沿用现有成果”的假设：先说明原因，按AGENTS.md只验证受影响部分；不要为绕过差异改写旧报告。默认本轮不需要启动Go测试。

workflow是本轮实际新修改，必须检查：

1. `git diff --check`和staged diff范围。
2. 使用已有actionlint检查三份workflow。旧工具为1.7.7，可从P/logs/stage2e-trace-only/10-actionlint.json查执行路径；路径失效时只定位已安装工具，不安装新环境。若缺少shellcheck/pyflakes，可如历史那样明确关闭它们并记录未验证，不能算它们通过。
3. 静态核对事件表：push=定向；文档push=无Go；同fork PR=定向；外部PR=不运行fork自定义jobs；manual false=定向；manual true=定向加重型。无需为这张简单表开发新测试框架。

只运行一次正常静态验证；若发现明确配置错误，修复后最多再验证一次。实际功能回归失败需保存首个失败、仅处理相关原因，不循环扩大任务。网络/权限失败保留本地结果，不伪造push或CI成功。

## 5. 更新简短工作记录并提交推送

在docs/simulate-v1/WORKLOG.md追加本轮改动分组、复用的历史验证、本轮静态检查、已知限制。保留：Legacy旧夹具兼容问题；Cancnun为NOT_VISITED / EXISTING_FIXTURE_KEY；最终无-short全库与完整平台验证未完成。不要把201案例通过写成每个fork键均通过。

按显式路径暂存每组，检查staged diff及删除内容中没有敏感信息、二进制、原始日志、大型fixture。复查所有待push提交，防止旧提交里删掉的秘密随历史上传。不要git add -A，不提交本地planning目录。这里只需检查本次新增历史及改动，不扫描Geth全部上游历史。

完成本轮逻辑提交后，**一次push推送全部新提交即可**，不用每提交一次都触发CI：

```text
git push origin HEAD:refs/heads/feat/simulate-v1-tracing
```

推送前再确认仓库/分支、无force且可fast-forward。远端有其他新提交时先只读判断，不自动覆盖或改写历史；如果能安全保留双方提交再合并，按实际冲突范围处理，否则保存本地commit并报告。推送失败不得换仓库或修改全局代理冒充成功。

推送后用ls-remote核对该分支SHA与本地HEAD一致。只查询本次最终SHA的CI状态，不需要等待或反复轮询所有jobs；queued/running/未启用Actions均如实报告，不能称为PASS。不自动创建PR、合并或触发重型CI，也不发消息给其他对话。

最终用简短中文提供：每组commit SHA与用途、最终remote SHA及分支、push结果、本轮检查和历史复用的区别、CI实际状态、保留缺口。工作树若还有未包含改动，列出并解释；不要reset/clean隐藏它们。回档建议使用明确目标commit或经用户要求git revert，不强推抹掉历史。

任务完成标准：现有工作有可回档的个人fork提交，普通push不再自动拉起重型全库矩阵，剩余限制有清楚记录。这里的完成不等于可合并上游或全库验证通过。

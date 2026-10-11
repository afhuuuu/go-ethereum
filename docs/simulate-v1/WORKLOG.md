# eth_simulateV1 tracing 工作记录

## 2026-10-11：精简验证流程，启用个人 fork 检查点

用户批准：开发阶段按影响范围验证，每轮实质修改留下 Git 提交并推送个人 fork，便于回档。完整检查集中在最终候选准备合并/发布时，不能把开发检查点说成全部验证通过。AGENTS.md 已替换旧的“每次 commit 前全部检查”规则；旧报告保留原样。

本次只修改 AGENTS.md 和本目录文档，未修改 Geth 实现、测试数据、workflow 或业务脚本。只检查文档 diff、范围和待推送内容，不启动 Go。代码改动仍待后续按 NEXT_TASK.md 分组提交。

### 最新工作评审

- 已读取工作对话“评估未上链 MEV 交易模拟”的最新完成轮次 `01a12412-446b-7001-93fa-dffaaaf990bb`，完成的是 Stage7E 恢复执行；Stage7F 尚未执行，取消继续执行其多副本/打包方案。
- Stage7E 历史结果：TestTransaction 实际启动一次、退出0；213测试名称，202pass（父1、案例201）、11原有skip、0fail。Cancnun 保留 NOT_VISITED / EXISTING_FIXTURE_KEY。
- 上轮对原始日志等30项独立检查已通过；本轮复用该评审，只读取小型报告/退出记录，并检查当前待提交文件。没有再次执行测试或扫描全部源码/fixture。
- 六个核心功能及测试文件与已评审候选原字节一致。三个workflow内容与候选一致，其中go.yml、validate_pr.yml仅存在CRLF/LF差别。logtracer.go虽然可能出现在status中，实际Git内容diff为空，不应加入新的功能修改。
- 待提交simulate-v1.yml会在每次push后继续执行多版本全库、keeper和386检查；应先改成“push定向检查，重型检查手动触发”，再随代码检查点发布。

### 已完成的功能及验证边界

eth_simulateV1原本已支持按调用顺序继承请求内状态；新增的是在同一次执行中收集逐笔trace。输入使用tracer/tracerConfig，输出使用calls[i].trace；支持callTracer、prestateTracer(diffMode)及受限mux。未增加公开receipt，未修改debug_traceCall或EVM共识规则。

历史证据包括trace功能/race、相关API与tracer回归、受控HTTP、构建、lint、生成检查、状态/区块/交易测试。它们对应不同阶段和源码组合，不能拼接成最终完整验证通过。

仍未完成：Legacy旧fixture与基线规则的不兼容处置、最终源码组合的官方无-short全库验证、完整keeper覆盖及个人fork跨平台CI结果。一个Cancnun未受检键仍须单列。开发检查点允许记录这些限制；合并/发布就绪声明不允许绕过它们。

### 本地证据索引（原始大文件不进Git）

根目录：`D:/Geth-newfunc-debug/geth-simulate-v1-planning`。

- `stage7e-ordinary-transaction-resume/REPORT.md`、`test.result.json`、`VALIDATION-STATUS.md`
- `stage7e-ordinary-transaction-resume-review/REVIEW.md`、`evidence.json`
- `stage7a-final-candidate/MANIFEST.json`、`VALIDATION.md`
- `stage7d-transaction-coverage/REPORT.md`、`test-helper.patch`
- `stage5b-legacy-observation-review/REVIEW.md`

功能原始基线：`c9a2bc73c847319a8faa57de59e42c0efc420682`。这不是后续工作的固定HEAD门禁；新提交自然会改变HEAD。今后用实际提交和Git diff跟踪变化。

### 交付方式

唯一推送仓库：`git@github.com:afhuuuu/go-ethereum.git`；固定分支：`feat/simulate-v1-tracing`。每轮可以有多个逻辑提交，完成该轮后一次push即可保留所有回档点，不必每保存一行或每个提交都触发CI。没有改动的只读评审不制造空提交。

本轮提交SHA和远端核对结果在完成后向用户汇报，不为了将提交自身SHA写进文档再创建循环提交。

## 2026-10-11：保存已评审实现和辅助修正，分离定向与完整 CI

这是开发检查点（WIP），不是合并或发布就绪。起点为已发布的文档提交 `99849f7568ba948251d4e2eb71d64450d1f88012`，源码原始基线仍为 `c9a2bc73c847319a8faa57de59e42c0efc420682`。

按独立逻辑保存：六文件模拟 tracing；trezor 生成 include 参数；Windows 多余空行；普通状态测试过时目录；三文件交易 fork/入口/回归修正；个人 fork CI；本工作记录。仅比较指定十二份 Go 文件与已评审目标：功能六文件匹配 Stage7A MANIFEST，前三份辅助文件匹配 Stage6C 固定来源，交易三文件匹配 Stage7D source-after。没有重新设计功能、修改 fixture、依赖或原生 tracer，也没有新增公开 receipt。

### 历史验证复用（本轮没有运行 Go）

| 来源及源码范围 | 历史结果 | 本轮使用边界 |
| --- | --- | --- |
| Stage2E trace-only 六文件，Stage4C 相关 API/tracer | 定向功能、生命周期 race、相关 API/tracer 回归退出 0 | 六份功能文件原字节相同；复用历史格式检查和功能证据 |
| Stage5C/5G Windows 修正及固定依赖 | 对应 lint、生成/tidy 检查退出 0 | 只复用已迁入的生成参数/空行修正，不宣称最终平台矩阵通过 |
| Stage6C corrected 状态入口 | TestState 109244 个叶通过；保留原过滤 | 入口修正原字节相同，排除项不算通过 |
| Stage7D 三文件交易工具 | RED 退出 1；GREEN 退出 0，717 个可执行 case 受检 | 三文件原字节相同；负向结果仍为失败证据 |
| Stage7E resume，Stage7D 修正后的源码 | TestTransaction 退出 0，201 个案例通过、11 个 skip | 不是所有 fork 结果键通过；保留 Cancnun 未受检键 |

这些结果使用各报告记录的 Go 1.27.1、输入版本和当时环境。只读来源/命令/退出码仍见原报告，不重跑或打包它们，不将不同源码组合的历史结果拼成最终完整验证通过。

### 本轮 CI 和静态检查

`simulate-v1.yml` 保留测试名称存在性检查、Linux/Windows 模拟及原生 tracer 回归、Linux race 和 geth 构建。重型 job 的条件同时要求个人仓库、workflow_dispatch 和布尔 full_validation=true；依赖顺序为两平台定向 → quality → 多版本全库 → keeper → 386。保留只读权限、PR 来源限制、凭据不持久化、既有版本/超时/并发取消。go.yml 和 validate_pr.yml 保留仅上游仓库执行的限制。

| 事件（目标为个人 fork） | 定向 Go jobs | 重型 jobs |
| --- | --- | --- |
| 普通 push，路径匹配 | 运行 | 不运行 |
| 仅文档/Markdown push 或 PR | 不运行 | 不运行 |
| 同 fork PR，路径匹配 | 运行 | 不运行 |
| 外部来源 PR | 不运行 fork 自定义 jobs | 不运行 |
| 手动 full_validation=false（默认） | 运行 | 不运行 |
| 手动 full_validation=true | 运行 | 依次运行，前置成功才继续 |

本轮 `git diff --check` 退出 0。正常 actionlint 只执行一次：

```text
D:/Geth-newfunc-debug/geth-simulate-v1-planning/tools/actionlint-1.7.7/actionlint.exe -shellcheck= -pyflakes= .github/workflows/go.yml .github/workflows/validate_pr.yml .github/workflows/simulate-v1.yml
```

actionlint 退出 **1（FAIL）**：go.yml 第 19、47、66、90 行各有两个既有自定义 runner 标签诊断（self-hosted-ghr 及 size-s-x64/size-l-x64），共 8 项；这些标签在本轮起点已经存在。validate_pr.yml 和 simulate-v1.yml 无诊断。未改写上游 runner 或检查规则，未重跑。shellcheck、pyflakes 明确关闭，状态为 NOT_RUN，不计通过。完整首轮输出保留在本轮执行记录；此处只存摘要，不把原始日志加入 Git。

提交前逐组检查 staged diff，并检查本轮所有新增历史（包含删除内容）；仅纳入源码、workflow 和工作记录，不纳入二进制、原始日志、大型 fixture 或本地 planning 目录。对新增文件和 diff 的敏感凭据模式检查无可疑命中；这不是对全部上游历史的安全审计。

### 保留限制与停止条件

Legacy 旧 fixture 兼容性未解决；Cancnun 仍为 NOT_VISITED / EXISTING_FIXTURE_KEY。最终源码组合的官方无-short全库、完整 keeper/平台覆盖和远端定向 CI 结果仍需独立确认。actionlint 的既有标签诊断仍保留；没有手动触发重型验证，没有重复 Go 测试、建立源码副本或打包。推送和远端 SHA/CI 快照在完成后单独汇报，不把 queued/running 写成 PASS。

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

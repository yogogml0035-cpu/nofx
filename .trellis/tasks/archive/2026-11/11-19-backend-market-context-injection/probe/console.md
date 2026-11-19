# C4 运行时验证记录（2026-11-20）

运行时验证以 Go 测试覆盖决策链注入路径与断链语义（探针截图在 C3 已覆盖看板 UI；本任务的注入发生在决策 prompt，非浏览器 UI）：

| # | 验收项 | 验证方式 | 结果 |
|---|--------|----------|------|
| 1 | MarketContext 序列化往返 | store/strategy_schema_test.go TestMarketContextRoundTrip（设置→Marshal 含键→Unmarshal→三标志保持；默认全关；grid marshal 不 panic） | PASS |
| 2 | EstimateTokens 计入新节 | store/strategy_schema_test.go TestEstimateTokensMarketContextIncluded（勾选后估算严格增大） | PASS |
| 3 | 决策 prompt 含注入段（勾选）/不含（未勾选） | kernel/market_env_test.go TestBuildUserPromptIncludesMarketEnvWhenEnabled（同快照两策略对比断言） | PASS |
| 4 | 注入节格式化（中英/截断/Unavailable/冷启动省略变化） | kernel/market_env_test.go 六用例 | PASS |
| 5 | gatemcp 断链（GATEMCP_ENDPOINT=127.0.0.1:9）决策照常组装 | trader/auto_trader_market_env_test.go TestBuildMarketEnvSnapshotUnavailable（三源 Unavailable 标记）+ TestUnavailableSnapshotRendersPlaceholder（prompt 含 [数据暂不可用] 且仍含段标题） | PASS |
| 6 | 全不勾选零调用 | kernel TestWriteMarketEnvDisabledOrNil + trader 层 if 守卫（MarketContextEnabled() false 不进入 fetch） | PASS |

说明：真实 AI 决策一轮的端到端（需真实模型 key）留待 C5 集成收口与送测执行；本任务以决策链组装与注入格式化单元/集成测试覆盖全部确定性路径。

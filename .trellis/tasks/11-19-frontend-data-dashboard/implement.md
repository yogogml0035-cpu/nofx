# C3 执行计划

前置：C1（/api/market/* 与 funding_rate 可用）、C2（DataPage iframe 已删）合入父分支后开工。

## 步骤

1. [ ] `lib/api/data.ts` 扩展：`getMarketOverview/getFearGreed/getMarketNews/getSentiment/getLiquidations` 五个客户端函数 + MarketSymbol 类型补 `funding_rate`
2. [ ] `components/data/BoardTable.tsx` 通用榜单（列配置 + 排序输入 + 加载/错误态）+ 单测
3. [ ] `components/data/` 各卡片组件（Overview/FearGreed/News/Sentiment/Liquidation）+ 单测
4. [ ] `DataPage.tsx` 重写：双栏布局、30s 轮询 hook（沿用现有项目内轮询模式，grep 现有实现保持一致）、手动刷新
5. [ ] i18n 键（zh/en 起步，结构对齐现有 translations）
6. [ ] vitest 用例补齐（排序切片、错误占位、轮询 hook）
7. [ ] `npm --prefix web run build && npm --prefix web run test` 全绿 → commit
8. [ ] 浏览器探针：起后端 + 前端，桌面/移动视口截图七模块；模拟单端点失败截图错误占位；留档 `probe/`

## 验证命令

```bash
npm --prefix web run build
npm --prefix web run test
```

## 回滚点

组件粒度分 commit；探针不通过不进父分支。

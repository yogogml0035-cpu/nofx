# C3 执行计划

前置：C1（/api/market/* 六端点与 funding_rate 可用）、C2（DataPage 已置空态占位）合入父分支后开工；C2b 为顺序前置（父分支串行）。

## 步骤

1. [ ] `lib/api/data.ts` 扩展：getMarketOverview/getFearGreed/getMarketNews/getAnnouncements/getSentiment/getLiquidations 客户端函数 + MarketSymbol 类型补 `funding_rate`
2. [ ] `components/data/BoardTable.tsx` 通用榜单（列配置 + 排序输入 + 加载/错误态）+ 单测
3. [ ] `components/data/` 各卡片组件（Overview/FearGreed/News 含公告/Sentiment/Liquidation）+ 单测
4. [ ] `DataPage.tsx` 重写：双栏布局、SWR 轮询（refreshInterval=30000、shouldRetryOnError:false）、手动刷新、上次成功时间前端记录
5. [ ] i18n 键补齐 en/zh/id 三语言
6. [ ] vitest 用例补齐（排序切片含 undefined→0、费率换算、错误占位、轮询参数）
7. [ ] `npm --prefix web run build && npm --prefix web run test` 全绿 → commit
8. [ ] 浏览器探针：起后端 + 前端，桌面/移动视口截图七模块；断链两组对比截图（GATEMCP_ENDPOINT 法，见 PRD）；留档 `probe/`

## 验证命令

```bash
npm --prefix web run build
npm --prefix web run test
```

## 回滚点

组件粒度分 commit；探针不通过不进父分支。

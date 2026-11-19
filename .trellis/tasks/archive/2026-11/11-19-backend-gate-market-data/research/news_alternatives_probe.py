# 测试 /mcp/news 的替代新闻通道 + search_news 的 coin/time_range/lang 参数形态
import json, urllib.request

URL = "https://api.gatemcp.ai/mcp/news"

def post(payload, session=None, timeout=60):
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }
    if session:
        headers["Mcp-Session-Id"] = session
    req = urllib.request.Request(URL, json.dumps(payload).encode(), headers, method="POST")
    try:
        resp = urllib.request.urlopen(req, timeout=timeout)
        return resp.status, resp.headers.get("Mcp-Session-Id"), resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, None, e.read().decode("utf-8", "replace")[:300]

def parse_sse(body):
    for line in body.splitlines():
        if line.startswith("data:"):
            try:
                return json.loads(line[5:].strip())
            except Exception:
                pass
    try:
        return json.loads(body)
    except Exception:
        return None

status, sid, body = post({
    "jsonrpc": "2.0", "id": 1, "method": "initialize",
    "params": {"protocolVersion": "2024-11-05", "capabilities": {},
               "clientInfo": {"name": "nofx-news-alt", "version": "0.1"}},
})
print(f"initialize: HTTP {status}")
post({"jsonrpc": "2.0", "method": "notifications/initialized"}, sid)

CALLS = [
    # search_news 补充参数形态
    ("search_news coin=BTC", "news_feed_search_news", {"query": "bitcoin", "coin": "BTC", "limit": 5}),
    ("search_news coin only", "news_feed_search_news", {"coin": "BTC", "limit": 5}),
    ("search_news time_range=7d", "news_feed_search_news", {"query": "bitcoin", "time_range": "7d", "limit": 5}),
    ("search_news lang=zh", "news_feed_search_news", {"query": "比特币", "lang": "zh", "limit": 5}),
    ("search_news platform", "news_feed_search_news", {"query": "bitcoin", "platform": "gate", "limit": 5}),
    # 替代通道（零参/最小参）
    ("latest_events 默认", "news_events_get_latest_events", {"limit": 5}),
    ("latest_events BTC", "news_events_get_latest_events", {"coin": "BTC", "limit": 5}),
    ("exchange_announcements gate", "news_feed_get_exchange_announcements", {"exchange": "gate", "limit": 5}),
    ("exchange_announcements 全部", "news_feed_get_exchange_announcements", {"limit": 5}),
    ("hot_topics BTC", "news_feed_get_hot_topics", {"coin": "BTC"}),
    ("web_search bitcoin", "news_feed_web_search", {"query": "bitcoin news today", "limit": 5}),
    ("search_ugc bitcoin", "news_feed_search_ugc", {"query": "bitcoin", "limit": 5}),
]

for label, tool, args in CALLS:
    status, _, body = post({"jsonrpc": "2.0", "id": 2, "method": "tools/call",
                            "params": {"name": tool, "arguments": args}}, sid)
    res = parse_sse(body)
    print(f"\n[{label}] HTTP {status}")
    if not res or "result" not in res:
        print(f"  raw: {body[:200]}")
        continue
    result = res["result"]
    data = result.get("structuredContent")
    if not data:
        try:
            data = json.loads(result.get("content", [{}])[0].get("text", "{}"))
        except Exception:
            data = {}
    # 摘要打印：对每类结构抽关键字段
    if isinstance(data, dict):
        keys = list(data.keys())
        print(f"  keys: {keys[:14]}")
        for k in ("total", "count", "items", "events", "announcements", "topics", "answer", "posts", "rows", "results"):
            if k in data:
                v = data[k]
                if isinstance(v, list):
                    print(f"  {k}: list[{len(v)}]")
                    if v:
                        first = v[0]
                        t = first.get("title") or first.get("headline") or first.get("content") or first.get("name") or str(first)[:100]
                        print(f"    首条: {str(t)[:110]}")
                else:
                    print(f"  {k}: {str(v)[:110]}")
    else:
        print(f"  {str(data)[:200]}")

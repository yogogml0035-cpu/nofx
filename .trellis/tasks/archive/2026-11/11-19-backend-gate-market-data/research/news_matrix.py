# news_feed_search_news 参数矩阵探针：找出能返回真实新闻的参数组合
# 背景：query="bitcoin" + 服务端默认(similarity_score="0.6", sort_by="time") 返回 total:0
import json, itertools, time, urllib.request

BASE = "https://api.gatemcp.ai/mcp/news"

def post(payload, session=None, timeout=30):
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }
    if session:
        headers["Mcp-Session-Id"] = session
    req = urllib.request.Request(BASE, json.dumps(payload).encode(), headers, method="POST")
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

# initialize 一个长会话，矩阵复用
status, sid, body = post({
    "jsonrpc": "2.0", "id": 1, "method": "initialize",
    "params": {"protocolVersion": "2024-11-05", "capabilities": {},
               "clientInfo": {"name": "nofx-news-matrix", "version": "0.1"}},
})
print(f"initialize: HTTP {status} session={'yes' if sid else 'no'}")
if status != 200:
    raise SystemExit("initialize failed: " + body[:200])
post({"jsonrpc": "2.0", "method": "notifications/initialized"}, sid)

def call_search(args):
    status, _, body = post({"jsonrpc": "2.0", "id": 2, "method": "tools/call",
                            "params": {"name": "news_feed_search_news", "arguments": args}}, sid, timeout=60)
    res = parse_sse(body)
    if not res or "result" not in res:
        return status, None, body[:120]
    text = res["result"].get("structuredContent") or {}
    if not text:
        # 回退：从 content[0].text 解析
        try:
            text = json.loads(res["result"]["content"][0]["text"])
        except Exception:
            text = {}
    return status, text, None

QUERIES = ["bitcoin", "BTC", "crypto", "比特币", "ethereum", "market"]
SIMS = ["0.3", "0.1", "0"]
SORTS = ["time", "total_score", "relevance"]

print(f"{'query':10} {'sim':6} {'sort':12} {'total':>6} {'count':>6}  首条标题")
print("-" * 100)
hits = []
for q, sim, sort in itertools.product(QUERIES, SIMS, SORTS):
    args = {"query": q, "limit": 20, "similarity_score": sim, "sort_by": sort}
    try:
        status, data, err = call_search(args)
    except Exception as e:
        print(f"{q:10} {sim:6} {sort:12}  EXC {e}")
        continue
    if data is None:
        print(f"{q:10} {sim:6} {sort:12}  HTTP {status} err={err}")
        continue
    total = data.get("total", "?")
    count = data.get("count", "?")
    items = data.get("items") or []
    first = (items[0].get("title") or "")[:40] if items else ""
    print(f"{q:10} {sim:6} {sort:12} {total:>6} {count:>6}  {first}")
    if isinstance(total, int) and total > 0:
        hits.append((q, sim, sort, total, count))

# 无 similarity_score / 无 sort_by 的最小参数形态
for q in ["bitcoin", "BTC", "crypto"]:
    args = {"query": q, "limit": 20}
    status, data, err = call_search(args)
    if data:
        print(f"{q:10} {'unset':6} {'unset':12} {data.get('total','?'):>6} {data.get('count','?'):>6}  {((data.get('items') or [{}])[0].get('title') or '')[:40]}")
        if isinstance(data.get("total"), int) and data["total"] > 0:
            hits.append((q, "unset", "unset", data["total"], data["count"]))

print("\n===== 命中汇总（total>0）=====")
for h in hits:
    print(h)
if not hits:
    print("全矩阵 total=0：news 通道无可用参数组合")

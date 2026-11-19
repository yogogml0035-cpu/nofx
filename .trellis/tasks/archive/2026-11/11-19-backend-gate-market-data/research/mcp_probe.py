# MCP Streamable HTTP 探针：验证 api.gatemcp.ai /info /news 四工具可用性
import json, sys, urllib.request

BASE = "https://api.gatemcp.ai"

def post(url, payload, session=None, timeout=30):
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }
    if session:
        headers["Mcp-Session-Id"] = session
    req = urllib.request.Request(url, json.dumps(payload).encode(), headers, method="POST")
    try:
        resp = urllib.request.urlopen(req, timeout=timeout)
        sid = resp.headers.get("Mcp-Session-Id")
        body = resp.read().decode("utf-8", "replace")
        return resp.status, sid, body
    except urllib.error.HTTPError as e:
        return e.code, None, e.read().decode("utf-8", "replace")[:300]

def parse_sse(body):
    # Streamable HTTP 响应可能是 SSE 流：取 data: 行拼 JSON
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

def probe_server(path, calls):
    url = BASE + path
    print(f"\n===== {url} =====")
    status, sid, body = post(url, {
        "jsonrpc": "2.0", "id": 1, "method": "initialize",
        "params": {"protocolVersion": "2024-11-05",
                   "capabilities": {},
                   "clientInfo": {"name": "nofx-probe", "version": "0.1"}},
    })
    init = parse_sse(body)
    print(f"initialize: HTTP {status} session={'yes' if sid else 'no'} server={init.get('result',{}).get('serverInfo') if init else body[:200]}")
    if status != 200:
        return
    status, _, _ = post(url, {"jsonrpc": "2.0", "method": "notifications/initialized"}, sid)
    for tool, args in calls:
        status, _, body = post(url, {"jsonrpc": "2.0", "id": 2, "method": "tools/call",
                                     "params": {"name": tool, "arguments": args}}, sid, timeout=60)
        res = parse_sse(body)
        ok = None
        if res and "result" in res:
            ok = not res["result"].get("isError", False)
        preview = json.dumps(res, ensure_ascii=False)[:600] if res else body[:300]
        print(f"[{tool}] HTTP {status} ok={ok}")
        print(f"  {preview}")

probe_server("/mcp/info", [
    ("info_marketsnapshot_get_market_overview", {}),
    ("info_marketsnapshot_get_market_snapshot", {"symbol": "BTC"}),
])
probe_server("/mcp/news", [
    ("news_feed_search_news", {"query": "bitcoin", "limit": 5}),
    ("news_feed_get_social_sentiment", {"symbol": "BTC"}),
])

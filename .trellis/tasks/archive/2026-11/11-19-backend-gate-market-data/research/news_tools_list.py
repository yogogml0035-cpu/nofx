# 列出 /mcp/info 与 /mcp/news 服务器的全部工具（tools/list），并逐个试调非 search 类新闻工具
import json, urllib.request

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

for path in ["/mcp/info", "/mcp/news"]:
    url = "https://api.gatemcp.ai" + path
    status, sid, body = post(url, {
        "jsonrpc": "2.0", "id": 1, "method": "initialize",
        "params": {"protocolVersion": "2024-11-05", "capabilities": {},
                   "clientInfo": {"name": "nofx-tools-list", "version": "0.1"}},
    })
    print(f"\n===== {url} : initialize HTTP {status} =====")
    if status != 200:
        print(body[:200])
        continue
    post(url, {"jsonrpc": "2.0", "method": "notifications/initialized"}, sid)
    status, _, body = post(url, {"jsonrpc": "2.0", "id": 2, "method": "tools/list"}, sid)
    res = parse_sse(body)
    tools = (res or {}).get("result", {}).get("tools", [])
    for t in tools:
        print(f"- {t['name']}: {t.get('description','')[:120]}")
        schema = (t.get("inputSchema") or {}).get("properties", {})
        print(f"  params: {list(schema.keys())}")

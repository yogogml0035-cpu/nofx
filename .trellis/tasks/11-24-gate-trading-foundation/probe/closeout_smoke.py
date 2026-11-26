#!/usr/bin/env python3
"""父任务收口的串联冒烟（不使用 AI、不下单、不启动 trader）。

从当前候选构建隔离的 compose project（nofx-closeout-smoke，独立容器名、端口与数据卷），
串联验证：健康检查与前端入口 → 注册/登录 → Gate K 线（全部保留周期、1w/30d/7d 边界、
非保留周期 400）→ 24h 行情 → 策略创建/读取/旧周期归一化 → 提示词预览（Gate 数据、无金额字段）
→ 无 trader 被自动拉起。失败即停，退出码非 0；证据写到 probe/smoke-<sha>/。

用法：python3 closeout_smoke.py [--port 1189] [--keep]
依赖：宿主机 docker、python3；worktree 根有 .env（RSA 私钥与 JWT 配置经 env_file 注入）。
"""

import argparse
import base64
import json
import os
import secrets
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[4]
PROJECT = "nofx-closeout-smoke"
BACKEND = f"{PROJECT}-backend"
FRONTEND = PROJECT
RETAINED = ["10s", "1m", "5m", "15m", "30m", "1h", "4h", "8h", "1d", "7d", "1w", "30d"]
GOPROXY = "https://goproxy.cn,direct"

results = []


def sh(args, check=True, capture=True):
    proc = subprocess.run(args, cwd=ROOT, text=True,
                          stdout=subprocess.PIPE if capture else subprocess.DEVNULL,
                          stderr=subprocess.STDOUT if capture else subprocess.DEVNULL)
    if check and proc.returncode != 0:
        raise RuntimeError(f"command failed ({proc.returncode}): {' '.join(args)}\n{proc.stdout or ''}")
    return proc.stdout.strip() if capture else ""


def record(name, ok, detail=None):
    results.append({"step": name, "result": "PASS" if ok else "FAIL", "detail": detail or {}})
    print(("PASS " if ok else "FAIL ") + name, flush=True)
    if not ok:
        save_results()
        raise SystemExit(1)


def save_results():
    out_dir.mkdir(parents=True, exist_ok=True)
    (out_dir / "summary.json").write_text(json.dumps({
        "sha": SHA, "project": PROJECT, "port": PORT, "results": results,
        "verdict": "SMOKE_PASS" if results and all(r["result"] == "PASS" for r in results) else "SMOKE_FAIL",
    }, ensure_ascii=False, indent=2))


def http(method, path, body=None, token=None, expect=200):
    url = f"http://127.0.0.1:{PORT}{path}"
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            raw = resp.read()
            status = resp.status
    except urllib.error.HTTPError as e:
        raw = e.read()
        status = e.code
    text = raw.decode("utf-8", "replace")
    parsed = None
    try:
        parsed = json.loads(text)
    except ValueError:
        pass
    if status != expect:
        raise AssertionError(f"{method} {path} -> {status}, expected {expect}: {text[:300]}")
    return status, parsed, text


def container_health(name):
    try:
        return sh(["docker", "inspect", "-f", "{{.State.Health.Status}}", name])
    except RuntimeError:
        return ""


def main():
    global SHA, PORT, out_dir
    parser = argparse.ArgumentParser()
    parser.add_argument("--port", type=int, default=1189)
    parser.add_argument("--keep", action="store_true", help="失败时保留栈便于排查")
    args = parser.parse_args()
    PORT = args.port
    SHA = sh(["git", "rev-parse", "--short", "HEAD"])
    out_dir = ROOT / ".trellis/tasks/11-24-gate-trading-foundation/probe" / f"smoke-{SHA}"

    if not (ROOT / ".env").exists():
        raise SystemExit(f"missing {ROOT / '.env'} (worktree env for RSA key / JWT)")
    if shutil.which("docker") is None:
        raise SystemExit("docker not found")

    work = Path(tempfile.mkdtemp(prefix="nofx-smoke-"))
    jwt = secrets.token_urlsafe(48)
    enc = base64.b64encode(secrets.token_bytes(32)).decode()
    override = work / "override.yml"
    override.write_text(f"""name: {PROJECT}
services:
  nofx:
    image: {BACKEND}:{SHA}
    container_name: {BACKEND}
    ports: !reset []
    volumes: !override
      - {PROJECT}_data:/app/data
    environment:
      TZ: Asia/Shanghai
      API_SERVER_PORT: "8080"
      DB_TYPE: sqlite
      DB_PATH: /app/data/data.db
      JWT_SECRET: "{jwt}"
      DATA_ENCRYPTION_KEY: "{enc}"
  nofx-frontend:
    image: {FRONTEND}:{SHA}
    container_name: {FRONTEND}
    ports: !override
      - "127.0.0.1:{PORT}:80"
volumes:
  {PROJECT}_data:
""")
    compose = ["docker", "compose", "-p", PROJECT, "-f", str(ROOT / "docker-compose.yml"), "-f", str(override)]

    def cleanup():
        if args.keep and results and any(r["result"] == "FAIL" for r in results):
            print(f"--keep: stack kept for inspection (project {PROJECT}, port {PORT})", file=sys.stderr)
            return
        subprocess.run(compose + ["down", "-v", "--remove-orphans"], cwd=ROOT,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        subprocess.run(["docker", "rmi", "-f", f"{BACKEND}:{SHA}", f"{FRONTEND}:{SHA}"],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        shutil.rmtree(work, ignore_errors=True)

    try:
        print(f"building candidate {SHA} into isolated project {PROJECT} ...", flush=True)
        sh(compose + ["build", "--build-arg", f"GOPROXY={GOPROXY}"], check=True, capture=False)
        record("images built", True, {"backend": f"{BACKEND}:{SHA}", "frontend": f"{FRONTEND}:{SHA}"})

        sh(compose + ["up", "-d"], check=True, capture=False)
        deadline = time.time() + 180
        http_ok = False
        while time.time() < deadline:
            try:
                status, _, _ = http("GET", "/api/health", expect=200)
                status2, _, _ = http("GET", "/", expect=200)
                http_ok = status == 200 and status2 == 200
                break
            except (AssertionError, OSError):
                time.sleep(2)
        record("http health via frontend proxy", http_ok)
        deadline = time.time() + 120
        states = {}
        while time.time() < deadline:
            states = {BACKEND: container_health(BACKEND), FRONTEND: container_health(FRONTEND)}
            if all(v == "healthy" for v in states.values()):
                break
            time.sleep(2)
        record("container health (image HEALTHCHECK)", all(v == "healthy" for v in states.values()), states)

        status, reg, _ = http("POST", "/api/register",
                              {"email": "smoke@example.com", "password": "SmokePass12345!", "lang": "en"},
                              expect=200)
        _, login, _ = http("POST", "/api/login",
                           {"email": "smoke@example.com", "password": "SmokePass12345!"})
        token = login["token"]
        record("register + login", bool(token))

        # Gate K 线：全部保留周期
        kline_detail = {}
        for tf in RETAINED:
            _, bars, _ = http("GET", f"/api/klines?symbol=BTC_USDT&interval={tf}&limit=5")
            ok = isinstance(bars, list) and len(bars) == 5
            if ok:
                for b in bars:
                    ok = ok and all(k in b for k in ("openTime", "open", "high", "low", "close", "volume")) \
                        and b["high"] >= b["low"] > 0 and b["volume"] > 0 and b["quoteVolume"] == b["volume"]
                times = [b["openTime"] // 1000 for b in bars]
                ok = ok and times == sorted(times) and len(set(times)) == 5
                days = [datetime.fromtimestamp(t, timezone.utc) for t in times]
                if tf == "1w":
                    ok = ok and all(d.weekday() == 0 for d in days)
                elif tf == "30d":
                    ok = ok and all(d.day == 1 for d in days)
                elif tf == "7d":
                    ok = ok and all(d.weekday() == 3 for d in days)
                elif tf == "1d":
                    ok = ok and all((d.hour, d.minute) == (0, 0) for d in days)
            kline_detail[tf] = {"bars": len(bars) if isinstance(bars, list) else None, "ok": ok}
            record(f"klines {tf}", ok, kline_detail[tf])

        status, _, text = http("GET", "/api/klines?symbol=BTC_USDT&interval=3m&limit=5", expect=400)
        record("klines non-retained 3m rejected", status == 400, {"body": text[:120]})

        _, tickers, _ = http("GET", "/api/agent/tickers?symbols=BTCUSDT,ETHUSDT,SOLUSDT")
        ok = isinstance(tickers, list) and {t.get("symbol") for t in tickers} == {"BTCUSDT", "ETHUSDT", "SOLUSDT"}
        ok = ok and all(float(t["lastPrice"]) > 0 and float(t["volume"]) > 0 for t in tickers)
        record("gate 24h tickers", ok, {"count": len(tickers) if isinstance(tickers, list) else None})

        # 策略：新语义字段与旧周期归一化
        cfg = {
            "coin_source": {"static_coins": ["BTCUSDT"]},
            "indicators": {"klines": {"primary_timeframe": "1h", "primary_count": 30}},
            "risk_control": {"position_percent": 10, "min_position_size": 10},
        }
        _, created, _ = http("POST", "/api/strategies",
                             {"name": "closeout-smoke", "description": "chained smoke", "lang": "en", "config": cfg},
                             token=token, expect=200)
        sid = created["id"]
        _, got, _ = http("GET", f"/api/strategies/{sid}", token=token)
        saved = got.get("config", got)
        rc = saved.get("risk_control", {})
        ok = rc.get("position_percent") == 10 and rc.get("min_position_size") == 10 \
            and saved.get("indicators", {}).get("klines", {}).get("primary_timeframe") == "1h"
        record("strategy create + read back", ok, {"id": sid, "position_percent": rc.get("position_percent"),
                                                   "min_position_size": rc.get("min_position_size")})

        legacy_cfg = {"coin_source": {"static_coins": ["BTCUSDT"]},
                      "indicators": {"klines": {"primary_timeframe": "3m"}}}
        _, created2, _ = http("POST", "/api/strategies",
                              {"name": "closeout-smoke-legacy", "lang": "en", "config": legacy_cfg},
                              token=token, expect=200)
        _, got2, _ = http("GET", f"/api/strategies/{created2['id']}", token=token)
        saved2 = got2.get("config", got2)
        tf2 = saved2.get("indicators", {}).get("klines", {}).get("primary_timeframe")
        record("legacy 3m timeframe normalized into retained set", tf2 in RETAINED, {"saved": tf2})

        # 提示词预览：Gate 数据、无金额字段
        _, preview, _ = http("POST", "/api/strategies/preview-prompt",
                             {"config": saved, "account_principal": 1000, "prompt_variant": "balanced"},
                             token=token)
        prompt = preview.get("system_prompt", "")
        ok = "Volume(USDT)" in prompt and "1h" in prompt \
            and "position_size_usd" not in prompt and "risk_usd" not in prompt \
            and "position_size_value_multiplier" not in prompt
        record("preview prompt (Gate data, no amount fields)", ok, {"chars": len(prompt)})

        # 不启动 trader：没有 trader、没有自动拉起
        _, my_traders, _ = http("GET", "/api/my-traders", token=token)
        record("no trader created", isinstance(my_traders, list) and len(my_traders) == 0,
               {"count": len(my_traders) if isinstance(my_traders, list) else None})
        count = sh(["docker", "exec", BACKEND, "sqlite3", "/app/data/data.db",
                    "SELECT COUNT(*) FROM traders WHERE is_running=1;"])
        record("no trader auto-started (db)", count == "0", {"running": count})
        logs = sh(["docker", "logs", BACKEND], check=False)
        auto = [m for m in ("Auto-starting trader", "Trader auto-started successfully",
                            "Auto-restoring trader runtime") if m in logs]
        record("no trader auto-started (logs)", not auto, {"markers": auto})

        save_results()
        print("SMOKE_PASS", flush=True)
        return 0
    finally:
        cleanup()


if __name__ == "__main__":
    sys.exit(main())

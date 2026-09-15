#!/usr/bin/env python3
# ============================================================
# ops web 后端 - 零依赖 (仅 Python 标准库)
# 把 ops 命令包装成 HTTP API, 并托管前端页面
#
# 启动:  python3 ~/ops/server.py [端口]   (默认 8090)
# 访问:  http://<服务器IP>:8090
#
# 认证:  登录口令取自环境变量 OPS_PASSWORD;
#        未设置时启动会随机生成一个并打印到控制台。
# 多主机: 主机列表存于 hosts.json, 可在网页添加/删除/测试。
# ============================================================
import hashlib
import hmac
import json
import os
import re
import secrets
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse, parse_qs

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
OPS = os.path.join(SCRIPT_DIR, "ops")
INDEX = os.path.join(SCRIPT_DIR, "index.html")
HOSTS_FILE = os.path.join(SCRIPT_DIR, "hosts.json")

# 白名单: 只允许这些操作和服务名, 防止命令注入
ALLOWED_ACTIONS = {"start", "stop", "restart", "deploy", "rollback"}
SERVICE_RE = re.compile(r"^[a-z]+$")            # 服务名: 纯小写字母
ID_RE = re.compile(r"^[0-9]{1,3}$")             # 实例 id: 1-3 位数字
IMAGE_RE = re.compile(r"^[a-zA-Z0-9._/:-]+$")   # 镜像 tag
HOST_RE = re.compile(r"^[A-Za-z0-9_-]{1,40}$")  # 主机名(hosts.json 的 name)
# SSH 地址: user@host 或 host, host 可为 ip/域名
SSH_RE = re.compile(r"^([A-Za-z0-9._-]+@)?[A-Za-z0-9._-]+$")

# ---------- 认证 ----------
# 优先环境变量 OPS_PASSWORD; 否则随机生成(每次启动变化, 打印到控制台)
AUTH_PASSWORD = os.environ.get("OPS_PASSWORD", "").strip()
if not AUTH_PASSWORD:
    AUTH_PASSWORD = secrets.token_urlsafe(9)
    # 打印(立即刷新)并写入文件, 方便未设置 OPS_PASSWORD 时查看
    print(f"[auth] 未设置 OPS_PASSWORD, 本次随机登录口令: {AUTH_PASSWORD}", flush=True)
    try:
        pw_file = os.path.join(SCRIPT_DIR, ".ops_password")
        with open(pw_file, "w", encoding="utf-8") as f:
            f.write(AUTH_PASSWORD + "\n")
        os.chmod(pw_file, 0o600)
        print(f"[auth] 口令已写入 {pw_file} (权限 600)", flush=True)
    except Exception as e:
        print(f"[auth] 写入口令文件失败: {e}", flush=True)

SESSIONS = {}                       # token -> 过期时间戳
SESSION_TTL = 12 * 3600             # 会话有效期 12 小时
_SESS_LOCK = threading.Lock()


def new_session():
    token = secrets.token_urlsafe(24)
    with _SESS_LOCK:
        SESSIONS[token] = time.time() + SESSION_TTL
    return token


def valid_session(token):
    if not token:
        return False
    with _SESS_LOCK:
        exp = SESSIONS.get(token)
        if not exp:
            return False
        if exp < time.time():
            SESSIONS.pop(token, None)
            return False
        return True


def drop_session(token):
    with _SESS_LOCK:
        SESSIONS.pop(token, None)


# ---------- 主机列表 ----------
_HOSTS_LOCK = threading.Lock()


def load_hosts():
    """读取 hosts.json, 返回 list。始终保证包含 local。"""
    hosts = []
    if os.path.exists(HOSTS_FILE):
        try:
            with open(HOSTS_FILE, encoding="utf-8") as f:
                data = json.load(f)
            if isinstance(data, list):
                hosts = [h for h in data if isinstance(h, dict) and h.get("name")]
        except Exception:
            hosts = []
    # 保证 local 永远存在且排最前
    if not any(h.get("name") == "local" for h in hosts):
        hosts.insert(0, {"name": "local", "ssh": "local", "port": 22, "note": "控制机本身"})
    return hosts


def save_hosts(hosts):
    with open(HOSTS_FILE, "w", encoding="utf-8") as f:
        json.dump(hosts, f, ensure_ascii=False, indent=2)


def host_exists(name):
    return any(h.get("name") == name for h in load_hosts())


def mask_secrets(text):
    """对详情文本中的敏感信息脱敏, 避免密码/凭据在网页明文展示。"""
    if not text:
        return text
    text = re.sub(
        r"(--[\w-]*(?:password|passwd|secret|token)\s+)(\S+)",
        r"\1******", text, flags=re.IGNORECASE,
    )
    text = re.sub(r"(://[^:/\s]+:)([^@/\s]+)(@)", r"\1******\3", text)
    text = re.sub(
        r"(?im)^([\w.]*(?:PASSWORD|PASSWD|SECRET|TOKEN|APIKEY|API_KEY)[\w.]*=)(.+)$",
        r"\1******", text,
    )
    return text


def run_ops(args, host=None, timeout=60):
    """执行 ops 命令, host 非空且非 local 时加 --host。返回 (rc, out)。"""
    cmd = [OPS]
    if host and host != "local":
        cmd += ["--host", host]
    elif host == "local":
        cmd += ["--host", "local"]
    cmd += args
    try:
        p = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
        out = (p.stdout or "") + (p.stderr or "")
        out = re.sub(r"\x1b\[[0-9;]*m", "", out)  # 去 ANSI 颜色
        return p.returncode, out
    except subprocess.TimeoutExpired:
        return 1, "命令执行超时"
    except Exception as e:
        return 1, f"执行失败: {e}"


class Handler(BaseHTTPRequestHandler):
    # ---------- 基础工具 ----------
    def _send(self, code, body, ctype="application/json; charset=utf-8", extra_headers=None):
        if isinstance(body, (dict, list)):
            body = json.dumps(body, ensure_ascii=False)
        data = body.encode("utf-8") if isinstance(body, str) else body
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Cache-Control", "no-store")
        for k, v in (extra_headers or {}):
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *a):
        pass

    def _cookie_token(self):
        raw = self.headers.get("Cookie", "") or ""
        for part in raw.split(";"):
            part = part.strip()
            if part.startswith("ops_session="):
                return part[len("ops_session="):]
        return ""

    def _authed(self):
        return valid_session(self._cookie_token())

    def _read_json(self):
        length = int(self.headers.get("Content-Length", 0) or 0)
        raw = self.rfile.read(length) if length else b"{}"
        try:
            return json.loads(raw or b"{}")
        except json.JSONDecodeError:
            return None

    def _host_param(self, q_or_body, key="host"):
        """取 host 参数并校验存在于主机列表。返回 (host, error_response_or_None)。
        q_or_body 可为 parse_qs 的结果(值为 list)或 JSON body(值为标量)。"""
        raw = q_or_body.get(key, "local")
        if isinstance(raw, list):
            raw = raw[0] if raw else "local"
        host = str(raw or "local")
        if not HOST_RE.match(host):
            return None, "非法主机名"
        if not host_exists(host):
            return None, f"主机不存在: {host}"
        return host, None

    # ---------- GET ----------
    def do_GET(self):
        u = urlparse(self.path)
        if u.path in ("/", "/index.html"):
            return self._serve_index()

        # 未登录: 仅放行首页(上面已处理)与登录状态查询
        if u.path == "/api/me":
            return self._send(200, {"ok": True, "authed": self._authed()})

        if not self._authed():
            return self._send(401, {"ok": False, "error": "未登录"})

        if u.path == "/api/hosts":
            # 返回主机列表(不含敏感信息, ssh 地址保留供展示)
            return self._send(200, {"ok": True, "hosts": load_hosts()})

        if u.path == "/api/status":
            q = parse_qs(u.query)
            host, err = self._host_param(q)
            if err:
                return self._send(400, {"ok": False, "error": err})
            rc, out = run_ops(["status", "--json"], host=host)
            try:
                return self._send(200, {"ok": True, "services": json.loads(out)})
            except json.JSONDecodeError:
                return self._send(200, {"ok": False, "error": out})

        if u.path == "/api/stats":
            q = parse_qs(u.query)
            host, err = self._host_param(q)
            if err:
                return self._send(400, {"ok": False, "error": err})
            # docker stats 需要采样, 适当放宽超时
            rc, out = run_ops(["stats", "--json"], host=host, timeout=30)
            try:
                return self._send(200, {"ok": True, "stats": json.loads(out)})
            except json.JSONDecodeError:
                return self._send(200, {"ok": False, "error": out})

        if u.path == "/api/logs":
            q = parse_qs(u.query)
            host, err = self._host_param(q)
            if err:
                return self._send(400, {"ok": False, "error": err})
            svc = q.get("service", [""])[0]
            sid = q.get("id", ["1"])[0]
            if not SERVICE_RE.match(svc):
                return self._send(400, {"ok": False, "error": "非法服务名"})
            if not ID_RE.match(sid):
                sid = "1"
            rc, out = run_ops(["logs", svc, sid], host=host)
            return self._send(200, {"ok": rc == 0, "log": out})

        if u.path == "/api/inspect":
            q = parse_qs(u.query)
            host, err = self._host_param(q)
            if err:
                return self._send(400, {"ok": False, "error": err})
            svc = q.get("service", [""])[0]
            sid = q.get("id", ["1"])[0]
            if not SERVICE_RE.match(svc):
                return self._send(400, {"ok": False, "error": "非法服务名"})
            if not ID_RE.match(sid):
                sid = "1"
            rc, out = run_ops(["inspect", svc, sid], host=host)
            return self._send(200, {"ok": rc == 0, "detail": mask_secrets(out)})

        return self._send(404, {"ok": False, "error": "not found"})

    # ---------- POST ----------
    def do_POST(self):
        u = urlparse(self.path)

        # 登录不需要已有会话
        if u.path == "/api/login":
            body = self._read_json()
            if body is None:
                return self._send(400, {"ok": False, "error": "无效的 JSON"})
            pw = str(body.get("password", ""))
            if hmac.compare_digest(pw, AUTH_PASSWORD):
                token = new_session()
                cookie = f"ops_session={token}; HttpOnly; SameSite=Strict; Path=/; Max-Age={SESSION_TTL}"
                return self._send(200, {"ok": True}, extra_headers=[("Set-Cookie", cookie)])
            return self._send(401, {"ok": False, "error": "口令错误"})

        if u.path == "/api/logout":
            drop_session(self._cookie_token())
            expired = "ops_session=; HttpOnly; SameSite=Strict; Path=/; Max-Age=0"
            return self._send(200, {"ok": True}, extra_headers=[("Set-Cookie", expired)])

        # 其余接口都需要登录
        if not self._authed():
            return self._send(401, {"ok": False, "error": "未登录"})

        if u.path == "/api/hosts":
            return self._add_host()
        if u.path == "/api/hosts/test":
            return self._test_host()
        if u.path == "/api/action":
            return self._action()

        return self._send(404, {"ok": False, "error": "not found"})

    # ---------- DELETE ----------
    def do_DELETE(self):
        u = urlparse(self.path)
        if not self._authed():
            return self._send(401, {"ok": False, "error": "未登录"})
        if u.path == "/api/hosts":
            q = parse_qs(u.query)
            name = q.get("name", [""])[0]
            if not HOST_RE.match(name):
                return self._send(400, {"ok": False, "error": "非法主机名"})
            if name == "local":
                return self._send(400, {"ok": False, "error": "不能删除 local"})
            with _HOSTS_LOCK:
                hosts = load_hosts()
                new = [h for h in hosts if h.get("name") != name]
                if len(new) == len(hosts):
                    return self._send(404, {"ok": False, "error": "主机不存在"})
                save_hosts(new)
            return self._send(200, {"ok": True})
        return self._send(404, {"ok": False, "error": "not found"})

    # ---------- 业务实现 ----------
    def _add_host(self):
        body = self._read_json()
        if body is None:
            return self._send(400, {"ok": False, "error": "无效的 JSON"})
        name = str(body.get("name", "")).strip()
        ssh = str(body.get("ssh", "")).strip()
        note = str(body.get("note", "")).strip()[:100]
        try:
            port = int(body.get("port", 22))
        except (TypeError, ValueError):
            return self._send(400, {"ok": False, "error": "端口必须是数字"})
        if not HOST_RE.match(name):
            return self._send(400, {"ok": False, "error": "主机名只能是字母/数字/-/_，长度 1-40"})
        if name == "local" or not SSH_RE.match(ssh):
            return self._send(400, {"ok": False, "error": "SSH 地址格式非法，应为 user@ip"})
        if not (1 <= port <= 65535):
            return self._send(400, {"ok": False, "error": "端口范围 1-65535"})
        with _HOSTS_LOCK:
            hosts = load_hosts()
            if any(h.get("name") == name for h in hosts):
                return self._send(409, {"ok": False, "error": "主机名已存在"})
            hosts.append({"name": name, "ssh": ssh, "port": port, "note": note})
            save_hosts(hosts)
        return self._send(200, {"ok": True})

    def _test_host(self):
        """测试连通性: 支持对已存在主机(按 name)或临时地址测试。"""
        body = self._read_json()
        if body is None:
            return self._send(400, {"ok": False, "error": "无效的 JSON"})
        name = str(body.get("name", "")).strip()
        if name:
            if not HOST_RE.match(name) or not host_exists(name):
                return self._send(400, {"ok": False, "error": "主机不存在"})
            rc, out = run_ops(["host-check"], host=name, timeout=20)
            return self._send(200, {"ok": rc == 0, "output": out.strip()})
        # 临时地址测试(添加前预检): 写入临时 hosts 文件不安全, 这里直接校验格式并用 ssh 探测
        ssh = str(body.get("ssh", "")).strip()
        try:
            port = int(body.get("port", 22))
        except (TypeError, ValueError):
            port = 22
        if not SSH_RE.match(ssh) or not (1 <= port <= 65535):
            return self._send(400, {"ok": False, "error": "SSH 地址或端口非法"})
        cmd = [
            "ssh", "-p", str(port), "-o", "BatchMode=yes",
            "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=8",
            ssh, "docker version --format '{{.Server.Version}}'",
        ]
        try:
            p = subprocess.run(cmd, capture_output=True, text=True, timeout=20)
            out = ((p.stdout or "") + (p.stderr or "")).strip()
            ok = p.returncode == 0
            msg = f"连接成功 (docker server {out})" if ok else f"连接失败: {out}"
            return self._send(200, {"ok": ok, "output": msg})
        except subprocess.TimeoutExpired:
            return self._send(200, {"ok": False, "output": "连接超时"})
        except Exception as e:
            return self._send(200, {"ok": False, "output": f"测试失败: {e}"})

    def _action(self):
        body = self._read_json()
        if body is None:
            return self._send(400, {"ok": False, "error": "无效的 JSON"})
        host, err = self._host_param(body)
        if err:
            return self._send(400, {"ok": False, "error": err})

        action = str(body.get("action", ""))
        service = str(body.get("service", ""))
        sid = str(body.get("id", "1"))
        image = str(body.get("image", "")).strip()

        if action not in ALLOWED_ACTIONS:
            return self._send(400, {"ok": False, "error": f"不支持的操作: {action}"})
        if not SERVICE_RE.match(service) and service != "all":
            return self._send(400, {"ok": False, "error": "非法服务名"})
        if not ID_RE.match(sid):
            return self._send(400, {"ok": False, "error": "非法实例 id"})
        if image and not IMAGE_RE.match(image):
            return self._send(400, {"ok": False, "error": "非法镜像名"})

        if action in ("start", "stop", "restart"):
            args = [action, service, sid]
        elif action == "deploy":
            args = ["deploy", service, sid] + ([image] if image else [])
        else:  # rollback
            if not image:
                return self._send(400, {"ok": False, "error": "回滚必须指定镜像"})
            args = ["rollback", service, image, sid]

        rc, out = run_ops(args, host=host, timeout=120)
        return self._send(200, {"ok": rc == 0, "output": out})

    def _serve_index(self):
        try:
            with open(INDEX, "rb") as f:
                self._send(200, f.read(), "text/html; charset=utf-8")
        except FileNotFoundError:
            self._send(500, "index.html 不存在", "text/plain; charset=utf-8")


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8090
    srv = ThreadingHTTPServer(("0.0.0.0", port), Handler)
    print(f"ops web 已启动: http://0.0.0.0:{port}  (Ctrl+C 停止)")
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        print("\n已停止")


if __name__ == "__main__":
    main()

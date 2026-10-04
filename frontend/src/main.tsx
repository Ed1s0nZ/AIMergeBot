import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  ShieldCheck,
  LayoutDashboard,
  FolderGit2,
  ScanLine,
  Users,
  Settings,
  ScrollText,
  LogOut,
  ArrowUpRight,
  ChevronRight,
} from "lucide-react";
import { api, write, type User } from "./api";
import { ErrorBox } from "./components";
import {
  Overview,
  Runs,
  RunDetail,
  Projects,
  UserAdmin,
  Events,
} from "./pages";
import { SystemSettings } from "./settings";
import "./style.css";
import "./responsive.css";
import "./workspace.css";
import "./workspace-responsive.css";

function Login({ onLogin }: { onLogin: (u: User) => void }) {
  const [username, setUsername] = useState(""),
    [password, setPassword] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <main className="login">
      <section className="login-form">
        <div className="form-wrap">
          <a className="login-brand" href="#/">
            <span>
              <ShieldCheck size={25} />
            </span>
            AIMergeBot
          </a>
          <span className="eyebrow">团队代码安全工作台</span>
          <h2>登录工作台</h2>
          <p className="muted">使用管理员为你创建的团队账号。</p>
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              setBusy(true);
              setError("");
              try {
                onLogin(
                  await api<User>(
                    "/auth/login",
                    write("POST", { username, password }),
                  ),
                );
              } catch (e) {
                setError((e as Error).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            <label>
              账号
              <input
                autoComplete="username"
                required
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </label>
            <label>
              密码
              <input
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </label>
            <ErrorBox error={error} />
            <button disabled={busy} className="primary full">
              {busy ? "正在登录…" : "进入工作台"}
              <ArrowUpRight size={17} />
            </button>
          </form>
          <p className="footnote">账号不可用？请联系团队管理员。</p>
          <div className="login-assurance">
            <ShieldCheck size={15} />
            <span>提交快照 · 证据审计 · 人工复核</span>
          </div>
        </div>
      </section>
    </main>
  );
}
function App() {
  const [user, setUser] = useState<User | null>(null),
    [loading, setLoading] = useState(true),
    [sessionError, setSessionError] = useState(""),
    [route, setRoute] = useState(location.hash.slice(1) || "/"),
    [logoutError, setLogoutError] = useState("");
  useEffect(() => {
    api<User>("/auth/me")
      .then(setUser)
      .catch((e) => {
        if (e.status !== 401) setSessionError(e.message);
      })
      .finally(() => setLoading(false));
    const hash = () => setRoute(location.hash.slice(1) || "/");
    const expired = () => setUser(null);
    window.addEventListener("hashchange", hash);
    window.addEventListener("session-expired", expired);
    return () => {
      window.removeEventListener("hashchange", hash);
      window.removeEventListener("session-expired", expired);
    };
  }, []);
  if (loading) return <div className="boot">正在连接工作台…</div>;
  if (!user)
    return (
      <>
        <ErrorBox error={sessionError} />
        <Login
          onLogin={(u) => {
            setUser(u);
            setSessionError("");
          }}
        />
      </>
    );
  const links = [
    { path: "/", label: "概览", icon: LayoutDashboard },
    { path: "/runs", label: "审计任务", icon: ScanLine },
    { path: "/projects", label: "项目", icon: FolderGit2 },
    ...(user.role === "admin"
      ? [
          { path: "/users", label: "团队成员", icon: Users },
          { path: "/settings", label: "系统设置", icon: Settings },
          { path: "/events", label: "操作日志", icon: ScrollText },
        ]
      : []),
  ];
  const detail = route.match(/^\/runs\/(\d+)$/);
  const title =
    links.find((x) => x.path === route)?.label ||
    (detail ? "审计详情" : "页面");
  return (
    <div className="shell">
      <aside className="sidebar">
        <a href="#/" className="brand">
          <ShieldCheck /> AIMergeBot
        </a>
        <span className="workspace-label">审计工作空间</span>
        <nav>
          {links.map(({ path, label, icon: Icon }) => (
            <a
              key={path}
              href={"#" + path}
              aria-current={
                route === path || (path === "/runs" && detail)
                  ? "page"
                  : undefined
              }
              className={
                route === path || (path === "/runs" && detail) ? "active" : ""
              }
            >
              <Icon size={18} />
              {label}
              <ChevronRight size={14} />
            </a>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="avatar">{user.username[0].toUpperCase()}</div>
          <div>
            <strong>{user.username}</strong>
            <small>{user.role === "admin" ? "管理员" : "团队成员"}</small>
          </div>
          <button
            title="退出登录"
            className="icon-button"
            onClick={async () => {
              try {
                await api("/auth/logout", write("POST"));
                setUser(null);
              } catch (e) {
                setLogoutError((e as Error).message);
              }
            }}
          >
            <LogOut size={17} />
          </button>
        </div>
      </aside>
      <div className="workspace">
        <header className="topbar">
          <span>
            AIMergeBot <span className="slash">/</span> {title}
          </span>
          <span className="topbar-tag">
            <span className="dot" /> 团队审计工作台
          </span>
        </header>
        <main className="content">
          <ErrorBox error={logoutError} />
          {detail ? (
            <RunDetail id={Number(detail[1])} />
          ) : route === "/" ? (
            <Overview />
          ) : route === "/runs" ? (
            <Runs />
          ) : route === "/projects" ? (
            <Projects admin={user.role === "admin"} />
          ) : route === "/users" && user.role === "admin" ? (
            <UserAdmin />
          ) : route === "/settings" && user.role === "admin" ? (
            <SystemSettings />
          ) : route === "/events" && user.role === "admin" ? (
            <Events />
          ) : (
            <div>
              页面不存在或没有权限。<a href="#/">返回概览</a>
            </div>
          )}
        </main>
        <footer>
          提交固定 · 证据可查 · 人工复核 <span>AIMergeBot</span>
        </footer>
      </div>
    </div>
  );
}
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
